package drawer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"image"
	"image/color"
	"image/png"
)

const (
	// rootUrl  = "https://www.oi-search.com/paintboard"
	rootUrl  = "https://pbdv.ishpduwu.link/paintboard"
	boardUrl = rootUrl + "/board"
	paintUrl = rootUrl + "/paint"
	tokenUrl = rootUrl + "/gettoken"
)

const (
	WIDTH  = 1000
	HEIGHT = 600
)

var board [WIDTH * HEIGHT]int
var boardLock *sync.Mutex

func init() {
	boardLock = new(sync.Mutex)
}

func getPixel(x int, y int) int {
	return board[x*HEIGHT+y]
}

func byteToHex(b byte) int {
	if '0' <= b && b <= '9' {
		return int(b - '0')
	} else {
		return int(b - 'a' + 10)
	}
}

func hexToByte(c int) byte {
	if c < 10 {
		return byte('0' + c)
	} else {
		return byte('a' + c - 10)
	}
}

func pixelToHex(rgb int) string {
	bs := make([]byte, 6)
	bs[5] = hexToByte((rgb >> 0) & 0xf)
	bs[4] = hexToByte((rgb >> 4) & 0xf)
	bs[3] = hexToByte((rgb >> 8) & 0xf)
	bs[2] = hexToByte((rgb >> 12) & 0xf)
	bs[1] = hexToByte((rgb >> 16) & 0xf)
	bs[0] = hexToByte((rgb >> 20) & 0xf)
	return string(bs)
}

func getBoard(force bool) {
	if !force {
		boardLock.Lock()
		defer func() {
			go func() {
				time.Sleep(UPDATE_INTERVAL * time.Second)
				boardLock.Unlock()
			}()
		}()
	}

	resp, err := http.Get(boardUrl)
	if err != nil {
		log.Println("Could not get board!")
		return
	}
	defer resp.Body.Close()

	f, err := os.OpenFile("board.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println("Error Open Board !", err)
		return
	}
	defer f.Close()

	reader := bufio.NewReader(resp.Body)

	for i := 0; i < WIDTH; i++ {
		// n, err := resp.Body.Read(buffer)
		// log.Println("Line Read", n)
		// log.Printf("Line Starts with '%s' Ends with '%s'\n", buffer[:12], buffer[n - 12 : n])
		buffer, err := reader.ReadBytes('\n')
		if err == io.EOF {
			break
		}

		if err != nil || len(buffer) != 3601 {
			log.Println("UKE !!!")
			log.Println(err)
			return
		}

		f.Write(buffer)
		for j := 0; j < HEIGHT; j++ {
			rgb := 0
			for k := 0; k < 6; k++ {
				rgb |= byteToHex(buffer[j*6+5-k]) << (4 * k)
			}
			board[i*HEIGHT+j] = rgb
		}
		if i%50 == 0 {
			log.Println("Line ", i, "done")
		}
	}
}

func saveBoard(fp io.Writer) error {
	img := image.NewRGBA(image.Rect(0, 0, WIDTH, HEIGHT))

	for i := 0; i < WIDTH; i++ {
		for j := 0; j < HEIGHT; j++ {
			pix := board[i*HEIGHT+j]
			img.Set(i, j, color.NRGBA{
				R: uint8(pix >> 16),
				G: uint8((pix >> 8) & 0xFF),
				B: uint8(pix & 0xFF),
				A: 255,
			})
		}
	}

	return png.Encode(fp, img)
}

type TokenResp struct {
	Status int    `json:"status"`
	Data   string `json:"data"`
}

func ParseResp(bs []byte) (token TokenResp) {
	err := json.Unmarshal(bs, &token)

	if err != nil {
		log.Println("Error: ", err)
	}

	return
}

// Token like dfe4d610-70c0-4fe6-b196-9b0e09ac920b
func getToken(uid int, paste string) (bool, string) {
	// s := log.Sprintf("uid=%v&paste=%v", uid, paste)
	// body := strings.NewReader(s)
	// resp, err := http.Post(tokenUrl, "x-www-form-urlencoded", body)

	body := url.Values{"uid": {strconv.Itoa(uid)}, "paste": {paste}}
	resp, err := http.PostForm(tokenUrl, body)

	if err != nil {
		log.Println("Could not get Token")
		return false, err.Error()
	}

	bs, _ := io.ReadAll(resp.Body)
	log.Println(string(bs))

	tok := ParseResp(bs)
	tok.Status = resp.StatusCode
	if !strings.Contains(string(bs), "200") {
		return false, tok.Data
	}
	log.Println("Get ok!")
	return true, tok.Data
}

func setPixel(x, y, c, uid int, token string) bool {
	body := url.Values{"x": {strconv.Itoa(x)}, "y": {strconv.Itoa(y)}, "color": {pixelToHex(c)}, "uid": {strconv.Itoa(uid)}, "token": {token}}
	log.Println("Set", body)
	resp, err := http.PostForm(paintUrl, body)

	if err != nil {
		log.Println("Counld not set Pixel:", err)
		return false
	}

	bs, _ := io.ReadAll(resp.Body)
	// log.Println(string(bs))

	tok := ParseResp(bs)
	if !strings.Contains(string(bs), "200") {
		log.Printf("UKE: %v\n", tok.Data)
		log.Println("Origin Message: ", string(bs))
		return false
	}
	log.Println("Ok at", x, y, pixelToHex(c))
	return true
}

type Api struct {
	cache map[int]string
	lock  *sync.RWMutex
}

func NewApi() *Api {
	return &Api{make(map[int]string), new(sync.RWMutex)}
}

func (api *Api) Update(force bool) {
	log.Println("Updating...")
	getBoard(force)
	log.Println("Update Done !")
}

func (api *Api) SaveBoard(f io.Writer) error {
	return saveBoard(f)
}

func (api *Api) GetPixel(x, y int) int {
	return getPixel(x, y)
}

func (api *Api) getCache(uid int) (string, bool) {
	api.lock.RLock()
	tok, ok := api.cache[uid]
	api.lock.RUnlock()
	return tok, ok
}

func (api *Api) setCache(uid int, tok string) {
	api.lock.Lock()
	api.cache[uid] = tok
	api.lock.Unlock()
}

func (api *Api) ClearTokens() {
	api.lock.Lock()
	api.cache = make(map[int]string)
	api.lock.Unlock()

	api.SaveToken()
}

func (api *Api) SaveToken() {
	f, err := os.OpenFile("_api.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()

	fmt.Fprintln(f, len(api.cache))

	api.lock.RLock()
	defer api.lock.RUnlock()

	for k, v := range api.cache {
		fmt.Fprintln(f, k, v)
	}
}

func (api *Api) ReadToken() {
	f, err := os.Open("_api.txt")
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()

	var n, uid int
	var tok string

	fmt.Fscan(f, &n)
	for i := 0; i < n; i++ {
		fmt.Fscan(f, &uid, &tok)
		api.setCache(uid, tok)
		log.Println("Cache ", uid, tok)
	}
}

func (api *Api) GetToken(uid int, paste string) (bool, string) {
	tok, ok := api.getCache(uid)
	if ok {
		return ok, tok
	}

	ok, tok = getToken(uid, paste)
	if ok {
		api.setCache(uid, tok)
		api.SaveToken()
	}
	return ok, tok
}

func (api *Api) GetTokenOrEmpty(uid int, paste string) string {
	tok, ok := api.getCache(uid)
	if ok {
		return tok
	}

	ok, tok = getToken(uid, paste)
	if ok {
		api.setCache(uid, tok)
		api.SaveToken()
		return tok
	}
	return ""
}

func (api *Api) SetPixel(x, y, c, uid int, token string) bool {
	return setPixel(x, y, c, uid, token)
}
