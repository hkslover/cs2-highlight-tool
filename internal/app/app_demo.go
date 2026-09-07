package app

import (
	"cs2-highlight-tool-v2/internal/demo"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) PickDemoFiles() ([]string, error) {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 Demo 文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "Demo Files (*.dem)", Pattern: "*.dem"},
		},
	})
	if err != nil {
		return nil, err
	}
	return a.prepareRawDemoFiles(paths)
}

func (a *App) ParseDemoFile(path string) (*demo.Metadata, error) {
	releaseFiles, fileErr := a.beginManagedFileUse()
	if fileErr != nil {
		return nil, fileErr
	}
	defer releaseFiles()

	if path == "" {
		return nil, nil
	}
	return demo.ParseMetadata(path)
}
