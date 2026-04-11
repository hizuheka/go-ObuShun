package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.design/x/hotkey"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	shcore                       = syscall.NewLazyDLL("shcore.dll")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procMonitorFromPoint         = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfo           = user32.NewProc("GetMonitorInfoW")
	procGetScaleFactorForMonitor = shcore.NewProc("GetScaleFactorForMonitor")
	procFindWindow               = user32.NewProc("FindWindowW")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
)

type point struct {
	x, y int32
}

type rect struct {
	left, top, right, bottom int32
}

type monitorInfo struct {
	size    uint32
	monitor rect
	work    rect
	flags   uint32
}

type targetMonitor struct {
	hMonitor uintptr
	work     rect
}

const (
	monitorDefaultToNearest = 0x00000002
)

func getMonitorScale(hMonitor uintptr) float64 {
	var scale uint32
	ret, _, _ := procGetScaleFactorForMonitor.Call(hMonitor, uintptr(unsafe.Pointer(&scale)))
	if ret != 0 || scale == 0 {
		return 1.0
	}
	return float64(scale) / 100.0
}

// Item represents a search result item matching the Svelte frontend expectations
type Item struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Source      string   `json:"source"`
	Args        []string `json:"args"`
	HistoryKey  *string  `json:"history_key"`
	Description string   `json:"description,omitempty"`
}

// App struct
type App struct {
	ctx       context.Context
	isVisible bool
	mu        sync.Mutex
	bookmarks []Item
	monitor   string
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		isVisible: false,
		monitor:   "cursor",
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	go a.listenForHotkeys()
	a.loadChromeBookmarks()
}

func (a *App) listenForHotkeys() {
	hk := hotkey.New([]hotkey.Modifier{hotkey.ModAlt}, hotkey.KeySpace)
	if err := hk.Register(); err != nil {
		log.Printf("Failed to register hotkey: %v\n", err)
		return
	}
	defer hk.Unregister()

	for {
		<-hk.Keydown()
		a.toggleWindow()
	}
}

func (a *App) toggleWindow() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.isVisible {
		a.hideWindow()
	} else {
		a.showWindow()
	}
}

func (a *App) hideWindow() {
	// 単純に隠すだけにする（ここで最小化すると2回目以降に画面外に消えるバグが起きるため）
	wailsRuntime.WindowHide(a.ctx)
	a.isVisible = false
}

func (a *App) showWindow() {
	// 1. 位置を計算して移動させる
	a.positionWindowNative()

	// 2. 以前フォーカス問題を完全に解決した「最小化・最小化解除」ハックを復活させる
	wailsRuntime.WindowShow(a.ctx)
	wailsRuntime.WindowMinimise(a.ctx)
	wailsRuntime.WindowUnminimise(a.ctx)

	// 3. フロントエンド通知
	wailsRuntime.EventsEmit(a.ctx, "show-launcher")

	wailsRuntime.WindowExecJS(a.ctx, `setTimeout(() => { 
			let el = document.querySelector('.search') || document.querySelector('.args-input'); 
			if(el) el.focus(); 
		}, 50);`)

	a.isVisible = true
}

func (a *App) positionWindowNative() {
	if runtime.GOOS != "windows" {
		return
	}

	// 1. Wailsのウィンドウハンドル(HWND)をタイトルから取得
	titlePtr, err := syscall.UTF16PtrFromString("go-obushun")
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if hwnd == 0 {
		return
	}

	// 2. 現在のマウスカーソルの物理座標を取得
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// 3. カーソルがあるモニターの物理座標（WorkArea）を取得
	packedPt := uintptr(uint32(pt.x)) | uintptr(uint32(pt.y))<<32
	hMonitor, _, _ := procMonitorFromPoint.Call(packedPt, monitorDefaultToNearest)
	if hMonitor == 0 {
		return
	}

	var mi monitorInfo
	mi.size = uint32(unsafe.Sizeof(mi))
	ret, _, _ := procGetMonitorInfo.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		return
	}

	// 4. ウィンドウの現在の論理サイズを取得
	logicalW, _ := wailsRuntime.WindowGetSize(a.ctx)
	if logicalW > 800 || logicalW < 100 {
		logicalW = 620
	}

	// 5. ターゲットモニターのスケールを取得し、移動後の物理的なウィンドウ幅を予測する
	scale := getMonitorScale(hMonitor)
	futurePhysicalW := int32(float64(logicalW) * scale)

	// 6. ターゲットモニターの物理的な幅と高さ
	monW := mi.work.right - mi.work.left
	monH := mi.work.bottom - mi.work.top

	// 7. 物理座標でのターゲット位置（中央・上から20%）を計算
	targetX := mi.work.left + (monW-futurePhysicalW)/2
	targetY := mi.work.top + int32(float64(monH)*0.2)

	// 8. ネイティブAPIで移動 (SWP_NOSIZE=0x0001, SWP_NOZORDER=0x0004)
	procSetWindowPos.Call(hwnd, 0, uintptr(targetX), uintptr(targetY), 0, 0, 0x0001|0x0004)
}

func (a *App) HideWindow() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hideWindow()
}

func (a *App) SetWindowSize(w int, h int) {
	wailsRuntime.WindowSetSize(a.ctx, w, h)
}

func (a *App) loadChromeBookmarks() {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return
	}
	bookmarkPath := filepath.Join(localAppData, "Google", "Chrome", "User Data", "Default", "Bookmarks")

	data, err := os.ReadFile(bookmarkPath)
	if err != nil {
		log.Printf("Failed to read bookmarks: %v\n", err)
		return
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		log.Printf("Failed to parse bookmarks: %v\n", err)
		return
	}

	var parsedBookmarks []Item

	var parseNode func(node map[string]interface{})
	parseNode = func(node map[string]interface{}) {
		if node["type"] == "url" {
			name, _ := node["name"].(string)
			url, _ := node["url"].(string)
			if name != "" && url != "" {
				parsedBookmarks = append(parsedBookmarks, Item{
					Name:   name,
					Path:   url,
					Source: "Bookmark",
					Args:   []string{},
				})
			}
		} else if node["type"] == "folder" {
			children, ok := node["children"].([]interface{})
			if ok {
				for _, child := range children {
					if childMap, ok := child.(map[string]interface{}); ok {
						parseNode(childMap)
					}
				}
			}
		}
	}

	roots, ok := root["roots"].(map[string]interface{})
	if ok {
		for _, rootNode := range roots {
			if rootMap, ok := rootNode.(map[string]interface{}); ok {
				parseNode(rootMap)
			}
		}
	}

	a.mu.Lock()
	a.bookmarks = parsedBookmarks
	a.mu.Unlock()
}

func (a *App) SearchItems(query string, searchMode string, sortOrder string) []Item {
	a.mu.Lock()
	defer a.mu.Unlock()

	results := []Item{}
	queryLower := strings.ToLower(query)

	for _, b := range a.bookmarks {
		if query == "" || strings.Contains(strings.ToLower(b.Name), queryLower) || strings.Contains(strings.ToLower(b.Path), queryLower) {
			results = append(results, b)
		}
		if len(results) >= 50 {
			break
		}
	}

	return results
}

func (a *App) LaunchItem(item map[string]interface{}, extraArgs []string) error {
	path, ok := item["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("invalid path")
	}

	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	case "darwin":
		err = exec.Command("open", path).Start()
	default:
		err = exec.Command("xdg-open", path).Start()
	}

	if err != nil {
		log.Printf("Failed to open URL: %v\n", err)
		return err
	}

	a.isVisible = false
	wailsRuntime.WindowHide(a.ctx)

	return nil
}

func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
