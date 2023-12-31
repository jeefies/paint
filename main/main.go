package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	draw "jeefy/drawer"
)

var api *draw.Api
var drawer *draw.ImageDrawer

func init() {
	api = draw.NewApi()
	drawer = draw.NewDrawer(api)
}

func AddToken() {
	var uid int
	var paste string

	fmt.Println("UID? ")
	fmt.Scanf("%d", &uid)
	fmt.Println("Paste? ")
	fmt.Scanf("%s", &paste)

	ok, tok := api.GetToken(uid, paste)
	if !ok {
		fmt.Println("Failed!")
		return
	}

	drawer.AddToken(uid, tok)
	fmt.Println("OK!")
}

func FixToken() {
	var uid int
	var tok string

	fmt.Println("UID? ")
	fmt.Scanf("%d", &uid)
	fmt.Println("Token? ")
	fmt.Scanf("%s", &tok)

	drawer.AddToken(uid, tok)
	fmt.Println("OK")
}

func SetImage() {
	var path string
	fmt.Println("Path? ")
	fmt.Scanf("%s", &path)

	if _, err := os.Stat(path); err != nil {
		fmt.Println(err)
		return
	}

	err := drawer.SetImage(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("OK!")
}

func SetX() {
	var nx int
	fmt.Println("X?")
	fmt.Scanf("%d\n", &nx)

	if nx < 0 || nx > 1000 {
		fmt.Println("Invalid !")
	} else {
		drawer.X = nx
		drawer.Reset()
		fmt.Println("Set ok !")
	}
}

func SetY() {
	var ny int
	fmt.Println("Y?")
	fmt.Scanf("%d\n", &ny)

	if ny < 0 || ny > 600 {
		fmt.Println("Invalid !")
	} else {
		drawer.Y = ny
		drawer.Reset()
		fmt.Println("Set ok !")
	}
}

func PrintPixel() {
	var t, x, y int
	fmt.Println("Type, X, Y ? ")
	fmt.Scanln(&t, &x, &y)
	if t == 0 {
		fmt.Printf("%#X\n", api.GetPixel(x, y))
	} else if t == 1 {
		fmt.Printf("%#X\n", drawer.GetPixel(x, y))
	} else if t == 2 {
		f, err := os.Create("board.png")
		if err != nil {
			fmt.Println(err)
			return
		}

		err = api.SaveBoard(f)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
}

func StartDraw() {
	drawer.Start()
}

func readConfig(f io.Reader, drawer *draw.ImageDrawer) {
	log.Println("Reading Config")
	var path, ignore string
	fmt.Fscanln(f, &path, &ignore)
	log.Println("Image Path: ", path, "[ignore: ", ignore, "]")
	var x, y int
	fmt.Fscanln(f, &x, &y)
	log.Println("Put AT ", x, y)

	if ignore == "ignore" {
		drawer.SetIgnore(true)
	}

	drawer.X = x
	drawer.Y = y
	drawer.SetImage(path)

	var n int
	fmt.Fscanln(f, &n)
	log.Println("Extra token count:", n)

	for i := 0; i < n; i++ {
		for {
			var uid int
			var paste string
			_, err := fmt.Fscan(f, &uid, &paste)
			if err == io.EOF {
				return
			} else if err != nil {
				fmt.Println("Error: ", err)
				return
			}

			fmt.Println("Wait...Reading Token for", uid)
			ok, tok := api.GetToken(uid, paste)
			if !ok {
				fmt.Println("??? ", uid, paste, "failed")
				return
			}
			drawer.AddToken(uid, tok)
			fmt.Println("Token", uid, "fetched", tok, "!")
		}
	}

	log.Println("Read Done")
}

func main() {
	api.ReadToken()

	if len(os.Args) > 2 && os.Args[1] == "task" {
		f, err := os.Open(os.Args[2])
		if err != nil {
			log.Println("Could not Open file: ", os.Args[2])
			return
		}
		defer f.Close()
		runner := NewTaskRunner(api)
		runner.ReadTasks(f)
		runner.Mainloop()
		return
	}

	func() {
		f, err := os.Open("config.txt")
		if err != nil {
			fmt.Println(err)
			return
		}
		defer f.Close()
		readConfig(f, drawer)
	}()

	if len(os.Args) > 1 && os.Args[1] == "start" {
		time.Sleep(3 * time.Second)
		StartDraw()
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(">>> ")
		opt, _ := reader.ReadString('\n')
		if len(opt) < 1 {
			continue
		}

		if opt[0] == 'a' {
			AddToken()
		} else if opt[0] == 'f' {
			FixToken()
		} else if opt[0] == 'i' {
			SetImage()
		} else if opt[0] == 'x' {
			SetX()
		} else if opt[0] == 'y' {
			SetY()
		} else if opt[0] == 's' {
			StartDraw()
		} else if opt[0] == 'p' {
			PrintPixel()
		} else if opt[0] == 'u' {
			api.Update(true)
		} else {
			fmt.Println("帮助：")
			fmt.Println("输入 h 获取帮助")
			fmt.Println("输入 a / add 新增 token,之后会有提示")
			fmt.Println("输入 i / image 设置图片")
			fmt.Println("输入 x / y 设置图片位置")
			fmt.Println("输入 s / start 开始绘制")
			fmt.Println()
			fmt.Println("当前信息：")
			fmt.Println("图片：", drawer.ImgPath)
			fmt.Println("位置：", drawer.X, drawer.Y)
			fmt.Println("可用 UID:")
			for k := range drawer.GetTokens() {
				fmt.Print(k, ' ')
			}
			fmt.Println()
			continue
		}
	}
}
