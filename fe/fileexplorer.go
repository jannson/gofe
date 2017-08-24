package fe

import (
	"mime/multipart"

	"github.com/md2k/gofe/models"
)

type FileExplorer interface {
	Init() error
	ListDir(path string) ([]models.ListDirEntry, error)
	Rename(path string, newPath string) error
	Move(path []string, newPath string) (err error)
	Copy(path []string, newPath string, singleFilename string) (err error)
	Delete(path []string) (err error)
	Chmod(path []string, code string, recursive bool) (err error)
	Mkdir(path string) error
	UploadFile(destination string, part *multipart.Part) (err error)
	Close() error
	GetContent(item string) (string, error)
	Edit(item, content string) error
}
