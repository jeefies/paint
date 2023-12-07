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
	tokens map[int] string
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
	draw.tokens = make(map[int] string)
	draw.waited = make(chan int, WAIT_BUF)
	draw.uncert = make([]bool, UNCERT_LEN)
	draw.unused = make(chan int, UNUSED_BUF)
	draw.ctx, draw.cancelFunc = nil, nil
	return draw
}

func (draw *ImageDrawer) AddToken(uid int, tok string) {
	draw.tokens[uid] = tok
	draw.unused <- uid
}

func (draw *ImageDrawer) Reset() {
	if draw.cancelFunc != nil {
		draw.cancelFunc()
		draw.ctx, draw.cancelFunc = nil, nil
	}

	draw.waited = nil
	draw.unused = nil
	for i := range draw.uncert {
		draw.uncert[i] = false
	}
	draw.waited = make(chan int, WAIT_BUF)
	draw.unused = make(chan int, UNUSED_BUF)
	for k := range draw.tokens {
		fmt.Println("Unused ", k)
		draw.unused <- k
	}
}

// need check exists !
func (draw *ImageDrawer) SetImage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer f.Close()
	draw.Reset()
	draw.ImgPath = path

	draw.img, _, err = image.Decode(f)
	if err != nil {
		fmt.Println(err)
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

func (draw *ImageDrawer) work() {
	ImY := draw.img.Bounds().Dy()
	for {
		v, ok := <-draw.waited
		if (!ok) {
			return
		}
		draw.uncert[v] = false
		uid := <-draw.unused
		x, y := v / ImY, v % ImY
		r, g, b, _ := draw.img.At(x, y).RGBA()
		r, g, b = r >> 8, g >> 8, b >> 8
		// fmt.Println("Try Setting ", draw.X + x, draw.Y, r, g, b)
		ok = draw.api.SetPixel(x + draw.X, y + draw.Y, int((r << 16) | (g << 8) | b), uid, draw.tokens[uid])
		if ok {
			if rem := len(draw.waited); rem != 0 {
				fmt.Println("Still ", rem, "pixels in queue... >=", rem * INTERVAL / len(draw.tokens), "s")
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
	return draw.tokens
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
