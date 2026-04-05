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

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.design/x/hotkey"
)

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
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		isVisible: false,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.listenForHotkeys()
	// 初期起動時にブックマークを読み込んでおく
	a.loadChromeBookmarks()
}

// listenForHotkeys registers a global hotkey and waits for keydown events
func (a *App) listenForHotkeys() {
	// Register Alt + Space
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

// toggleWindow handles the show/hide logic
func (a *App) toggleWindow() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.isVisible {
		// 隠すときは最小化してから隠す
		wailsRuntime.WindowMinimise(a.ctx)
		wailsRuntime.WindowHide(a.ctx)
		a.isVisible = false
	} else {
		// 初回起動時など、最小化状態でない場合でも確実に「最小化からの復帰」アクションを
		// Windowsに認識させるため、表示する直前に一瞬だけ最小化状態を明示的に作る
		wailsRuntime.WindowMinimise(a.ctx)
		
		// 表示するときは表示してから「最小化解除」を呼ぶと、Windowsが強制的にアクティブにする
		wailsRuntime.WindowShow(a.ctx)
		wailsRuntime.WindowUnminimise(a.ctx)
		
		// Send the show event for frontend to reset its state
		wailsRuntime.EventsEmit(a.ctx, "show-launcher")
		
		// Focus the input
		wailsRuntime.WindowExecJS(a.ctx, `setTimeout(() => { 
			let el = document.querySelector('.search') || document.querySelector('.args-input'); 
			if(el) el.focus(); 
		}, 50);`)
		
		a.isVisible = true
	}
}

// loadChromeBookmarks reads the Chrome bookmarks JSON file
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

// SearchItems is called from Svelte to search items
func (a *App) SearchItems(query string, searchMode string, sortOrder string) []Item {
	a.mu.Lock()
	defer a.mu.Unlock()

	results := []Item{}
	queryLower := strings.ToLower(query)

	for _, b := range a.bookmarks {
		if query == "" || strings.Contains(strings.ToLower(b.Name), queryLower) || strings.Contains(strings.ToLower(b.Path), queryLower) {
			results = append(results, b)
		}
		if len(results) >= 50 { // 結果を50件までに制限して軽くする
			break
		}
	}

	return results
}

// LaunchItem is called from Svelte when an item is selected
func (a *App) LaunchItem(item map[string]interface{}, extraArgs []string) error {
	path, ok := item["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("invalid path")
	}

	// 選択されたアイテムがブックマークやURLの場合、デフォルトブラウザで開く
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

	// アプリを非表示にする
	a.isVisible = false
	wailsRuntime.WindowHide(a.ctx)

	return nil
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

