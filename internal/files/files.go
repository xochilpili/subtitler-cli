package file

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gen2brain/go-unarr"
	"golang.org/x/net/html/charset"
)

type File interface {
	ListFiles() []string
	ProcessSubtitles(path string) error
}

type file struct {
	filePath string
}

func New(filePath string) *file {
	return &file{
		filePath: filePath,
	}
}

func (f *file) ListFiles() []string {
	a, err := unarr.NewArchive(f.filePath)

	if err != nil {
		panic(fmt.Errorf("error while opening compressed file: %v", err))
	}
	defer a.Close()
	list, err := a.List()

	if err != nil {
		panic(fmt.Errorf("error listing file contents: %v", err))
	}

	// TODO: get only srt and ssa files.
	var subtitleFiles []string
	for _, item := range list {
		position := strings.LastIndex(item, ".")
		if position > -1 {
			extensionFile := item[position+1:]
			if extensionFile == "srt" || extensionFile == "ssa" {
				subtitleFiles = append(subtitleFiles, item)
			}
		}
	}
	return subtitleFiles
}

func (f *file) ProcessSubtitles(path string, clean bool) ([]*string, error) {
	// Extract files from compressed downloaded file.
	extractedFiles, err := f.extract(path)
	if err != nil {
		return nil, err
	}
	// Iterate over each extracted file and
	// determine if it's a subtitle based on it's extension
	// if it's not a subtitle the file will be removed
	// otherwise if it's a subtitle then validate encoded charset
	// and create a new file in UTF-8 encode
	var result []*string
	for _, subtitle := range extractedFiles {
		subtitlePath := filepath.Join(path, subtitle)
		extension := filepath.Ext(subtitlePath)
		if extension == ".srt" || extension == ".ssa" {
			err := f.fixCharset(subtitlePath)
			if err != nil {
				if errors.Is(err, io.EOF) {
					os.Remove(subtitlePath)
					continue
				}
				return nil, err
			}

			os.Rename(subtitlePath+".utf8", subtitlePath)
			result = append(result, &subtitlePath)
		} else {
			if _, err := os.Stat(subtitlePath); err == nil {
				err := os.Remove(subtitlePath)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	// remove compressed source file
	if clean {
		err := os.Remove(f.filePath)
		if err != nil {
			fmt.Printf("error while removing source %s, %v", f.filePath, err)
		}
	}

	return result, nil
}

func (f *file) extract(path string) ([]string, error) {
	a, err := unarr.NewArchive(f.filePath)
	if err != nil {
		panic(fmt.Errorf("unable to open file: %v", err))
	}
	defer a.Close()
	return a.Extract(path)
}

func (f *file) fixCharset(filename string) error {
	inputFile, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer inputFile.Close()

	// determine encoding of the file
	charsetReader, err := charset.NewReader(inputFile, "")
	if err != nil {
		return err
	}

	// Couldn't validate which charset the file is
	// therefor, every file will be encoded to UTF-8 :/

	// Read content with determined encoding
	scanner := bufio.NewScanner(charsetReader)
	scanner.Split(bufio.ScanLines)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	// connvert text to UTF-8
	utf8Bytes := []byte{}
	for _, line := range lines {
		utf8Bytes = append(utf8Bytes, []byte(line)...)
		utf8Bytes = append(utf8Bytes, '\n')
	}

	// Write the UTF-8 content to a new file
	outputFile := filename + ".utf8"
	err = os.WriteFile(outputFile, utf8Bytes, 0644)
	if err != nil {
		return err
	}

	return nil
}
