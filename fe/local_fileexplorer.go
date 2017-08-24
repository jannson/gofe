package fe

import (
	"bufio"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"github.com/md2k/gofe/models"
)

type LocalFileExplorer struct {
	FileExplorer
	RelativePath string
}

// Copies file source to destination dest.
func CopyFile(source string, dest string) (err error) {
	sf, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sf.Close()
	df, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer df.Close()
	_, err = io.Copy(df, sf)
	if err == nil {
		si, err := os.Stat(source)
		if err != nil {
			err = os.Chmod(dest, si.Mode())
		}

	}

	return
}

// Recursively copies a directory tree, attempting to preserve permissions.
// Source directory must exist, destination directory must *not* exist.
func CopyDir(source string, dest string) (err error) {

	// get properties of source dir
	fi, err := os.Stat(source)
	if err != nil {
		return err
	}

	if !fi.IsDir() {
		return &CustomError{"Source is not a directory"}
	}

	// ensure dest dir does not already exist

	_, err = os.Open(dest)
	if !os.IsNotExist(err) {
		return &CustomError{"Destination already exists"}
	}

	// create dest dir

	err = os.MkdirAll(dest, fi.Mode())
	if err != nil {
		return err
	}

	entries, err := ioutil.ReadDir(source)

	for _, entry := range entries {

		sfp := source + "/" + entry.Name()
		dfp := dest + "/" + entry.Name()
		if entry.IsDir() {
			err = CopyDir(sfp, dfp)
			if err != nil {
				log.Println(err)
			}
		} else {
			// perform copy
			err = CopyFile(sfp, dfp)
			if err != nil {
				log.Println(err)
			}
		}

	}
	return
}

// A struct for returning custom error messages
type CustomError struct {
	What string
}

// Returns the error message defined in What as a string
func (e *CustomError) Error() string {
	return e.What
}

func NewLocalFileExplorer(path string) *LocalFileExplorer {
	return &LocalFileExplorer{RelativePath: path}
}

func (fe *LocalFileExplorer) Init() error {
	return nil
}

func (fe *LocalFileExplorer) Mkdir(path string) error {
	return os.Mkdir(filepath.Join(fe.RelativePath, path), os.FileMode(int(0700)))
}

func (fe *LocalFileExplorer) ListDir(path string) ([]models.ListDirEntry, error) {
	files, err := ioutil.ReadDir(filepath.Join(fe.RelativePath, path))
	if err != nil {
		return nil, err
	}
	results := []models.ListDirEntry{}

	for _, f := range files {
		ftype := "file"
		if f.IsDir() {
			ftype = "dir"
		}
		results = append(results, models.ListDirEntry{
			Name:   f.Name(),
			Rights: f.Mode().String(),
			Size:   strconv.Itoa(int(f.Size())),
			Date:   f.ModTime().Format("2006-01-02 15:04:05"),
			Type:   ftype,
		})
	}

	return results, nil

}

func (fe *LocalFileExplorer) Rename(path string, newPath string) error {
	return os.Rename(filepath.Join(fe.RelativePath, path), filepath.Join(fe.RelativePath, newPath))
}

func (fe *LocalFileExplorer) Move(path []string, newPath string) (err error) {
	for _, target := range path {
		err = fe.Rename(target, newPath)
	}
	return err
}

func (fe *LocalFileExplorer) Copy(path []string, newPath string, singleFilename string) (err error) {
	for _, target := range path {
		err = CopyFile(filepath.Join(fe.RelativePath, target), filepath.Join(fe.RelativePath, newPath, singleFilename))
	}
	return err
}

func (fe *LocalFileExplorer) Delete(path []string) (err error) {
	for _, target := range path {
		err = os.RemoveAll(target)
	}
	return err
}

func (fe *LocalFileExplorer) Chmod(path []string, code string, recursive bool) (err error) {
	codeInt, err := strconv.Atoi(code)
	if err != nil {
		return
	}

	if recursive {
		for _, target := range path {
			filepath.Walk(filepath.Join(fe.RelativePath, target), func(name string, info os.FileInfo, err error) error {
				if err == nil {
					err = os.Chmod(name, os.FileMode(codeInt))
				}
				return err
			})
		}
	} else {
		for _, target := range path {
			err = os.Chmod(filepath.Join(fe.RelativePath, target), os.FileMode(codeInt))
		}
	}
	return err
}

func (fe *LocalFileExplorer) UploadFile(destination string, part *multipart.Part) (err error) {
	df, err := os.Create(filepath.Join(fe.RelativePath, destination, part.FileName()))
	if err != nil {
		return err
	}
	defer df.Close()
	_, err = io.Copy(df, part)
	return err
}

func (fe *LocalFileExplorer) Close() error {
	return nil
}

func (fe *LocalFileExplorer) GetContent(item string) (string, error) {
	realPath := filepath.Join(fe.RelativePath, item)
	fi, err := os.Stat(realPath)
	if err != nil {
		return "", err
	}
	if fi.Size() > 1024*1024 {
		return "", errors.New("file too big, not support getContent")
	}
	df, err := os.Open(realPath)
	if err != nil {
		return "", err
	}
	defer df.Close()

	dec := transform.NewReader(df, charmap.ISO8859_1.NewDecoder())
	//dec := bufio.NewReader(df)
	scanner := bufio.NewScanner(dec)
	texts := make([]string, 0)
	for scanner.Scan() {
		texts = append(texts, scanner.Text())
	}

	return strings.Join(texts, "\n"), nil
}

func (fe *LocalFileExplorer) Edit(item string, content string) error {
	fo, err := os.Create(filepath.Join(fe.RelativePath, item))
	if err != nil {
		return err
	}
	defer fo.Close()

	_, err = io.Copy(fo, strings.NewReader(content))
	return err
}
