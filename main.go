package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type Focus struct {
	window     fyne.Window
	tree       *widget.Tree
	editor     *CustomEditor
	currentDir string
	files      map[string][]string
	fontSize   float32
}

type CustomEditor struct {
	widget.Entry
	fontSize float32
	syntax   string
}

func NewCustomEditor() *CustomEditor {
	editor := &CustomEditor{
		fontSize: 12,
	}
	editor.ExtendBaseWidget(editor)
	editor.MultiLine = true
	editor.Wrapping = fyne.TextWrapWord
	editor.TextStyle = fyne.TextStyle{
		Monospace: true,
	}
	return editor
}

func (e *CustomEditor) SetFontSize(size float32) {
	e.fontSize = size
	e.Refresh()
}

func (e *CustomEditor) MinSize() fyne.Size {
	return e.Entry.MinSize()
}

func NewFocus() *Focus {
	return &Focus{
		files:    make(map[string][]string),
		fontSize: 12,
	}
}

var (
	goPatterns = map[string]*regexp.Regexp{
		"#00ADD8": regexp.MustCompile(`\b(func|package|import|return|if|else|for|range|var|type|struct|interface|map|chan|go|defer|select|case|default|break|continue|switch)\b`),
		"#FFA500": regexp.MustCompile(`"[^"]*"`),
		"#98C379": regexp.MustCompile(`//.*$`),
	}

	jsonPatterns = map[string]*regexp.Regexp{
		"#FFA500": regexp.MustCompile(`"[^"]*"`),
		"#00ADD8": regexp.MustCompile(`\b(true|false|null)\b`),
		"#98C379": regexp.MustCompile(`[{}\[\],]`),
	}

	xmlPatterns = map[string]*regexp.Regexp{
		"#00ADD8": regexp.MustCompile(`<[^>]+>`),
		"#FFA500": regexp.MustCompile(`"[^"]*"`),
		"#98C379": regexp.MustCompile(`<!--.*?-->`),
	}
)

func (f *Focus) applySyntaxHighlighting(text string, fileExt string) string {
	var patterns map[string]*regexp.Regexp

	switch fileExt {
	case ".go":
		patterns = goPatterns
	case ".json":
		patterns = jsonPatterns
	case ".xml":
		patterns = xmlPatterns
	default:
		return text
	}

	highlighted := text
	for color, pattern := range patterns {
		highlighted = pattern.ReplaceAllString(highlighted, fmt.Sprintf(`<span style="color: %s">$0</span>`, color))
	}

	return highlighted
}

func (f *Focus) updateFiles(root string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	f.currentDir = absRoot
	f.files = make(map[string][]string)
	f.files[f.currentDir] = []string{}

	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasPrefix(info.Name(), ".") || info.Name() == "node_modules" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}

		if absPath == absRoot {
			return nil
		}

		parentDir := filepath.Dir(absPath)
		f.files[parentDir] = append(f.files[parentDir], absPath)

		if info.IsDir() {
			f.files[absPath] = []string{}
		}

		return nil
	})

	return err
}

func (f *Focus) saveContent() {
	if f.editor == nil {
		return
	}

	content := f.editor.Text
	currentFile := f.window.Title()
	if currentFile != "Focus IDE" {
		err := os.WriteFile(currentFile, []byte(content), 0644)
		if err != nil {
			dialog.ShowError(err, f.window)
		}
	}
}

func (f *Focus) loadFile(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		dialog.ShowError(err, f.window)
		return
	}

	f.editor.syntax = filepath.Ext(path)
	f.editor.SetText(string(content))
	f.window.SetTitle(path)
}

func (f *Focus) createUI() {
	f.tree = &widget.Tree{
		ChildUIDs: func(uid string) []string {
			return f.files[uid]
		},
		IsBranch: func(uid string) bool {
			children, ok := f.files[uid]
			return ok && len(children) > 0
		},
		CreateNode: func(branch bool) fyne.CanvasObject {
			return widget.NewLabel("Template")
		},
		UpdateNode: func(uid string, branch bool, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			label.SetText(filepath.Base(uid))
		},
		OnSelected: func(uid string) {
			info, err := os.Stat(uid)
			if err != nil {
				dialog.ShowError(err, f.window)
				return
			}
			if !info.IsDir() {
				f.loadFile(uid)
			}
		},
	}

	f.editor = NewCustomEditor()
	f.editor.OnChanged = func(content string) {
		f.saveContent()
	}

	split := container.NewHSplit(
		container.NewScroll(f.tree),
		container.NewScroll(f.editor),
	)
	split.SetOffset(0.2)

	f.window.SetContent(split)

	// Handle keyboard shortcuts
	zoomInShortcut := &desktop.CustomShortcut{KeyName: fyne.KeyEqual, Modifier: desktop.ControlModifier}
	zoomOutShortcut := &desktop.CustomShortcut{KeyName: fyne.KeyMinus, Modifier: desktop.ControlModifier}

	f.window.Canvas().AddShortcut(zoomInShortcut, func(shortcut fyne.Shortcut) {
		f.editor.SetFontSize(f.editor.fontSize + 1)
	})

	f.window.Canvas().AddShortcut(zoomOutShortcut, func(shortcut fyne.Shortcut) {
		if f.editor.fontSize > 8 {
			f.editor.SetFontSize(f.editor.fontSize - 1)
		}
	})
}

func main() {
	focus := NewFocus()
	a := app.New()
	focus.window = a.NewWindow("Focus IDE")
	focus.window.Resize(fyne.NewSize(800, 600))

	path := "."
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		log.Fatal(err)
	}

	var dirPath string
	if fileInfo.IsDir() {
		dirPath = path
	} else {
		dirPath = filepath.Dir(path)
	}

	err = focus.updateFiles(dirPath)
	if err != nil {
		log.Fatal(err)
	}

	focus.createUI()

	absPath, _ := filepath.Abs(dirPath)
	focus.tree.Root = absPath
	focus.tree.Refresh()

	if !fileInfo.IsDir() {
		absFilePath, err := filepath.Abs(path)
		if err != nil {
			log.Fatal(err)
		}
		focus.loadFile(absFilePath)
	}

	focus.window.ShowAndRun()
}
