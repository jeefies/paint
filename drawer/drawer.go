package drawer

import (
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"
)

type DrawerError struct {
	msg string
}

func (err DrawerError) Error() string {
	return err.msg
}

const (
	INTERVAL        = 30
	WORKER_COUNT    = 7
	UNUSED_BUF      = 50
	RESET_BUF       = 100
	UNCERT_LEN      = 40000
	UPDATE_INTERVAL = 30 * 2
	WAIT_BUF        = 40000
	AHEADUP         = 2
	AHEAD           = 7
)

type ImageDrawer struct {
	api         *Api
	ImgPath     string
	img         image.Image
	X, Y        int
	ignoreWhite bool
	inque      []bool
	// pixels waiting to draw
	waited chan int
	// unused tokens
	unused     chan int
	ctx        context.Context
	cancelFunc context.CancelFunc
}

func NewDrawer(api *Api) *ImageDrawer {
	draw := &ImageDrawer{}
	draw.api = api
	draw.waited = make(chan int, WAIT_BUF)
	draw.inque = make([]bool, UNCERT_LEN)
	draw.unused = make(chan int, UNUSED_BUF)
	draw.ctx, draw.cancelFunc = nil, nil
	return draw
}

func (draw *ImageDrawer) AddToken(uid int, tok string) {
	draw.unused <- uid
}

func (draw *ImageDrawer) Reset() {
	log.Println("Reset...")
	if draw.cancelFunc != nil {
		draw.cancelFunc()
		draw.cancelFunc = nil
	}

	draw.waited = nil
	draw.unused = nil
	for i := range draw.inque {
		draw.inque[i] = false
	}
	draw.waited = make(chan int, WAIT_BUF)
	draw.unused = make(chan int, UNUSED_BUF)
	draw.api.lock.RLock()
	defer draw.api.lock.RUnlock()
	for k := range draw.api.cache {
		draw.unused <- k
	}
}

// need check exists !
func (draw *ImageDrawer) SetImage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer f.Close()

	draw.Reset()
	draw.ImgPath = path

	draw.img, _, err = image.Decode(f)
	if err != nil {
		return err
	}

	log.Println("Image Size: ", draw.img.Bounds())
	if draw.img.Bounds().Dx() > 200 || draw.img.Bounds().Dy() > 200 {
		return &DrawerError{"Too Large !!!"}
	}
	return nil
}

func (draw *ImageDrawer) SetIgnore(ignore bool) {
	draw.ignoreWhite = ignore
}

func (draw *ImageDrawer) ImageSize() (int, int) {
	return draw.img.Bounds().Dx(), draw.img.Bounds().Dy()
}

func (draw *ImageDrawer) GetPixel(x, y int) int {
	r, g, b, _ := draw.img.At(x, y).RGBA()
	r, g, b = r>>8, g>>8, b>>8
	return int((r << 16) | (g << 8) | b)
}

func (draw *ImageDrawer) WorkStatus() int {
	if draw.cancelFunc == nil {
		return -1
	}

	if rem := len(draw.waited); rem < 2 {
		return 0
	} else if len(draw.api.cache) == 0 {
		return -2
	} else {
		return rem * INTERVAL / len(draw.api.cache)
	}
}

func (draw *ImageDrawer) Start() {
	draw.Reset()
	draw.ctx, draw.cancelFunc = context.WithCancel(context.Background())

	lock, counter := new(sync.Mutex), new(int)

	go func() {
		log.Println("Available tokens: ")
		lock.Lock()
		for k, v := range draw.api.cache {
			log.Println(k, v)
		}
		lock.Unlock()
	}()

	draw.api.Update(true)
	go draw.check(draw.ctx)

	time.Sleep(5 * time.Second)
	for i := 0; i < WORKER_COUNT; i++ {
		go draw.work(lock, counter)
	}

	go func() {
		startTime := time.Now().Unix()
		for {
			timeout := make(chan int)
			go func() {
				time.Sleep(3 * time.Second)
				timeout <- 1
			}()

			select {
			case <-draw.ctx.Done():
				return
			case <-timeout:
			}

			if draw.WorkStatus() == 0 {
				log.Println("Start Maintaining...")
				return
			}

			curTime := time.Now().Unix()
			lock.Lock()
			log.Print("Token: ", len(draw.api.cache), " Rate:", float64(*counter*INTERVAL)/float64(int(curTime-startTime)*len(draw.api.cache)), "\r")
			lock.Unlock()
		}
	}()
}

func (draw *ImageDrawer) work(lock *sync.Mutex, counter *int) {
	ImY := draw.img.Bounds().Dy()
	var v int
	var ok bool
	for {
		select {
		case v, ok = <-draw.waited:
		case <-draw.ctx.Done():
			log.Println("Work Quit...")
			return
		}
		if !ok {
			log.Println("Work Quit...")
			return
		}
		draw.inque[v] = false
		x, y := v/ImY, v%ImY
		r, g, b, _ := draw.img.At(x, y).RGBA()
		r, g, b = r>>8, g>>8, b>>8
		// log.Println("Try Setting ", draw.X + x, draw.Y, r, g, b)
		exp := int((r << 16) | (g << 8) | b)
		if draw.ignoreWhite && exp == 0xFFFFFF {
			continue
		}

		uid := <-draw.unused
		tok, ok := draw.api.getCache(uid)
		if !ok {
			continue
		}

		ok = draw.api.SetPixel(x+draw.X, y+draw.Y, exp, uid, tok)
		if ok {
			if rem := len(draw.waited); rem != 0 {
				log.Println("Still ", rem, "pixels in queue... >=", rem*INTERVAL/len(draw.api.cache), "s")
			}
			go func() {
				lock.Lock()
				*counter += 1
				lock.Unlock()

				time.Sleep(time.Duration(INTERVAL)*time.Second - time.Second*AHEADUP/AHEAD)
				draw.unused <- uid
			}()
		} else {
			draw.unused <- uid
		}
	}
}

func (draw *ImageDrawer) GetTokens() map[int]string {
	draw.api.lock.RLock()
	defer draw.api.lock.RUnlock()

	copyed := make(map[int]string)
	for k, v := range draw.api.cache {
		copyed[k] = v
	}
	return copyed
}

func (draw *ImageDrawer) check(ctx context.Context) {
	timeout := make(chan int, 1)
	waitTime := time.Duration(UPDATE_INTERVAL)
	first := true
	for {
		go func() {
			time.Sleep(time.Second)
			timeout <- 1
		}()

		select {
		case <-timeout:
		case <-ctx.Done():
			log.Println("Check Quit...")
			return
		}

		x, y := draw.img.Bounds().Dx(), draw.img.Bounds().Dy()

		put := func(i, j int) {
			offset := i*y + j
			r, g, b, _ := draw.img.At(i, j).RGBA()
			r, g, b = r>>8, g>>8, b>>8
			exp := int((r << 16) | (g << 8) | b)
			if draw.ignoreWhite && exp == 0xFFFFFF {
				return
			}

			if exp != draw.api.GetPixel(draw.X+i, draw.Y+j) && !draw.inque[offset] {
				draw.inque[offset] = true
				log.Printf("Diff at %d, %d (to %d %d), expect %#x got %#x\n", i, j, i+draw.X, j+draw.Y, exp, draw.api.GetPixel(draw.X+i, draw.Y+j))
				draw.waited <- offset
			}
		}

		log.Println(first)
		if !first || len(draw.waited) <= 1000 {
			// for len(draw.waited) > len(draw.api.cache) {
			// 	draw.inque[<-draw.waited] = false
			// }

			log.Println("By Order")
			for offset := 0; offset < x*y; offset++ {
				i, j := offset/y, offset%y
				put(i, j)
			}
		} else {
			for _, offset := range rand.Perm(x * y) {
				i, j := offset/y, offset%y
				put(i, j)
			}
		}

		first = false
		log.Println("Draw Remain: ", len(draw.waited))
		go draw.api.Update(false)
		time.Sleep(waitTime * time.Second)
	}
}
