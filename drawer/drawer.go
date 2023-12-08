package drawer

import (
	"fmt"
	"image"
	"time"
	"os"
	"context"
	"math/rand"
	_ "image/jpeg"
	_ "image/png"
)

type DrawerError struct {
	msg string
}

func (err DrawerError) Error() string {
	return err.msg
}

const (
	INTERVAL = 5
	WORKER_COUNT = 5
	UNUSED_BUF = 50
	RESET_BUF = 100
	UNCERT_LEN = 40000
	UPDATE_INTERVAL = 60
	WAIT_BUF = 6000
)

type ImageDrawer struct {
	api *Api
	ImgPath string
	img image.Image
	X, Y int
	uncert []bool
	// pixels waiting to draw
	waited chan int
	// unused tokens
	unused chan int
	ctx context.Context
	cancelFunc context.CancelFunc
}

func NewDrawer(api *Api) (*ImageDrawer) {
	draw := &ImageDrawer{}
	draw.api = api
	draw.waited = make(chan int, WAIT_BUF)
	draw.uncert = make([]bool, UNCERT_LEN)
	draw.unused = make(chan int, UNUSED_BUF)
	draw.ctx, draw.cancelFunc = nil, nil
	return draw
}

func (draw *ImageDrawer) AddToken(uid int, tok string) {
	draw.unused <- uid
}

func (draw *ImageDrawer) Reset() {
	fmt.Println("Reset...")
	if draw.cancelFunc != nil {
		draw.cancelFunc()
		draw.cancelFunc = nil
	}

	draw.waited = nil
	draw.unused = nil
	for i := range draw.uncert {
		draw.uncert[i] = false
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

	fmt.Println("Image Size: ", draw.img.Bounds())
	if (draw.img.Bounds().Dx() > 200 || draw.img.Bounds().Dy() > 200) {
		return &DrawerError{"Too Large !!!"};
	}
	return nil
}

func (draw *ImageDrawer) ImageSize() (int, int) {
	return draw.img.Bounds().Dx(), draw.img.Bounds().Dy()
}

func (draw *ImageDrawer) Start() {
	draw.Reset()
	draw.ctx, draw.cancelFunc = context.WithCancel(context.Background())

	go draw.check(draw.ctx)
	for i := 0; i < WORKER_COUNT; i++ {
		go draw.work()
	}
}

func (draw *ImageDrawer) GetPixel(x, y int) int {
	r, g, b, _ := draw.img.At(x, y).RGBA()
	r, g, b = r >> 8, g >> 8, b >> 8
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

func (draw *ImageDrawer) work() {
	ImY := draw.img.Bounds().Dy()
	var v int
	var ok bool
	for {
		select {
			case v, ok = <- draw.waited:
			case <-draw.ctx.Done():
				fmt.Println("Work Quit...")
				return
		}
		if (!ok) {
			fmt.Println("Work Quit...")
			return
		}
		draw.uncert[v] = false
		uid := <-draw.unused
		x, y := v / ImY, v % ImY
		r, g, b, _ := draw.img.At(x, y).RGBA()
		r, g, b = r >> 8, g >> 8, b >> 8
		// fmt.Println("Try Setting ", draw.X + x, draw.Y, r, g, b)
		tok, ok := draw.api.getCache(uid)
		if !ok {
			continue
		}

		ok = draw.api.SetPixel(x + draw.X, y + draw.Y, int((r << 16) | (g << 8) | b), uid, tok)
		if ok {
			if rem := len(draw.waited); rem != 0 {
				fmt.Println("Still ", rem, "pixels in queue... >=", rem * INTERVAL / len(draw.api.cache), "s")
			}
			go func() {
				time.Sleep(time.Duration(INTERVAL) * time.Second + time.Duration(rand.Intn(100) - 500))
				draw.unused <- uid
			}()
		} else {
			draw.unused <- uid
		}
	}
}

func (draw *ImageDrawer) GetTokens() map[int] string {
	draw.api.lock.RLock()
	defer draw.api.lock.RUnlock()

	copyed := make(map[int] string)
	for k, v := range draw.api.cache {
		copyed[k] = v
	}
	return copyed
}

func (draw *ImageDrawer) check(ctx context.Context) {
	timeout := make(chan int, 1)
	waitTime := time.Duration(UPDATE_INTERVAL)
	for {
		go func() {
			time.Sleep(time.Second)
			timeout <- 1
		}()

		select {
		case <-timeout:
		case <-ctx.Done():
			fmt.Println("Check Quit...")
			break
		}

		draw.api.Update()
		x, y := draw.img.Bounds().Dx(), draw.img.Bounds().Dy()

		for _, offset := range rand.Perm(x * y) {
			i, j := offset / y, offset % y
			r, g, b, _ := draw.img.At(i, j).RGBA()
			r, g, b = r >> 8, g >> 8, b >> 8
			exp := int((r << 16) | (g << 8) | b)
			if exp != draw.api.GetPixel(draw.X + i, draw.Y + j) && !draw.uncert[offset] {
				draw.uncert[offset] = true
				fmt.Printf("Diff at %d, %d (to %d %d), expect %#x got %#x\n", i, j, i + draw.X, j + draw.Y, exp, draw.api.GetPixel(draw.X + i, draw.Y + j))
				draw.waited <- offset
			}
		}

		time.Sleep(waitTime * time.Second)
	}
}
