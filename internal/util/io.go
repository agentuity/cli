package util

import (
	"fmt"
	"io"
	"os"
	"path"
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
