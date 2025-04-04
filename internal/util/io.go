package util

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// Exists returns true if the filename or directory specified by fn exists.
func Exists(fn string) bool {
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return false
	}
	return true
}

// CopyFile will copy src to dst
func CopyFile(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}

// CopyDir will copy all files recursively from src to dst
func CopyDir(src string, dst string) error {
	var err error
	var fds []os.DirEntry
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return fmt.Errorf("error reading %s: %w", src, err)
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return fmt.Errorf("error mkdir %s: %w", dst, err)
	}

	if fds, err = os.ReadDir(src); err != nil {
		return fmt.Errorf("error readdir %s: %w", src, err)
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = CopyDir(srcfp, dstfp); err != nil {
				return fmt.Errorf("error copying directory from %s to %s: %w", srcfp, dstfp, err)
			}
		} else {
			if _, err = CopyFile(srcfp, dstfp); err != nil {
				return fmt.Errorf("error copying file from %s to %s: %w", srcfp, dstfp, err)
			}
		}
	}
	return nil
}

// ListDir will return an array of files recursively walking into sub directories
func ListDir(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	res := make([]string, 0)
	for _, file := range files {
		if file.IsDir() {
			newres, err := ListDir(filepath.Join(dir, file.Name()))
			if err != nil {
				return nil, err
			}
			res = append(res, newres...)
		} else {
			if file.Name() == ".DS_Store" {
				continue
			}
			res = append(res, filepath.Join(dir, file.Name()))
		}
	}
	return res, nil
}

// ZipDirCallbackMatcher is a function that returns true if the file should be included in the zip
type ZipDirCallbackMatcher func(fn string, fi os.FileInfo) bool

// ZipDir will zip up a directory into the outfilename and return an error if it fails
func ZipDir(dir string, outfilename string, opts ...ZipDirCallbackMatcher) error {
	zf, err := os.Create(outfilename)
	if err != nil {
		return fmt.Errorf("error opening: %s. %w", outfilename, err)
	}
	defer zf.Close()
	zw := zip.NewWriter(zf)
	defer zw.Close()
	files, err := ListDir(dir)
	if err != nil {
		return fmt.Errorf("error listing files: %w", err)
	}
	for _, file := range files {
		fn, err := filepath.Rel(dir, file)
		if err != nil {
			return fmt.Errorf("error getting relative path: %s. %w", file, err)
		}
		rf, err := os.Open(file)
		if err != nil {
			return fmt.Errorf("error opening file: %s. %w", file, err)
		}
		defer rf.Close()
		if len(opts) > 0 {
			fi, err := rf.Stat()
			if err != nil {
				return fmt.Errorf("error getting file info: %s. %w", file, err)
			}
			var notok bool
			for _, opt := range opts {
				if !opt(fn, fi) {
					rf.Close()
					notok = true
					break
				}
			}
			if notok {
				continue
			}
		}
		w, err := zw.Create(fn)
		if err != nil {
			return fmt.Errorf("error creating file: %s. %w", fn, err)
		}
		_, err = io.Copy(w, rf)
		if err != nil {
			return fmt.Errorf("error copying file: %s. %w", file, err)
		}
		rf.Close()
	}
	zw.Flush()
	zw.Close()
	return zf.Close()
}

func ReadFileLines(filename string, startLine, endLine int) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	lineNum := 0

	for scanner.Scan() {
		if lineNum >= startLine && (endLine < 0 || lineNum <= endLine) {
			lines = append(lines, scanner.Text())
		}
		lineNum++
		if endLine >= 0 && lineNum > endLine {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return lines, nil
}

func GetRelativePath(basePath, absolutePath string) string {
	if filepath.VolumeName(basePath) != filepath.VolumeName(absolutePath) && filepath.VolumeName(absolutePath) != "" {
		return filepath.ToSlash(absolutePath)
	}
	
	rel, err := filepath.Rel(basePath, absolutePath)
	if err != nil {
		return absolutePath
	}
	rel = filepath.ToSlash(rel)
	return rel
}
