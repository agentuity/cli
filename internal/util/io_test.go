package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "exists_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(tmpFile, []byte("test"), 0644)
	assert.NoError(t, err)

	subDir := filepath.Join(tmpDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	assert.NoError(t, err)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"existing file", tmpFile, true},
		{"existing directory", tmpDir, true},
		{"existing subdirectory", subDir, true},
		{"non-existing file", filepath.Join(tmpDir, "nonexistent.txt"), false},
		{"non-existing directory", filepath.Join(tmpDir, "nonexistentdir"), false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Exists(test.path)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "copyfile_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	srcContent := []byte("test content")
	srcFile := filepath.Join(tmpDir, "source.txt")
	err = os.WriteFile(srcFile, srcContent, 0644)
	assert.NoError(t, err)

	dstFile := filepath.Join(tmpDir, "destination.txt")

	t.Run("successful copy", func(t *testing.T) {
		n, err := CopyFile(srcFile, dstFile)
		assert.NoError(t, err)
		assert.Equal(t, int64(len(srcContent)), n)

		dstContent, err := os.ReadFile(dstFile)
		assert.NoError(t, err)
		assert.Equal(t, srcContent, dstContent)
	})

	t.Run("non-existent source", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "nonexistent.txt")
		_, err := CopyFile(nonExistentFile, dstFile)
		assert.Error(t, err)
	})

	t.Run("invalid destination", func(t *testing.T) {
		invalidDst := filepath.Join(tmpDir, "invalid", "destination.txt")
		_, err := CopyFile(srcFile, invalidDst)
		assert.Error(t, err)
	})

	t.Run("source is directory", func(t *testing.T) {
		_, err := CopyFile(tmpDir, dstFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not a regular file")
	})
}

func TestCopyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "copydir_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "source")
	err = os.Mkdir(srcDir, 0755)
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("file1"), 0644)
	assert.NoError(t, err)

	subDir := filepath.Join(srcDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("file2"), 0644)
	assert.NoError(t, err)

	dstDir := filepath.Join(tmpDir, "destination")

	t.Run("successful directory copy", func(t *testing.T) {
		err := CopyDir(srcDir, dstDir)
		assert.NoError(t, err)

		assert.True(t, Exists(dstDir))
		assert.True(t, Exists(filepath.Join(dstDir, "file1.txt")))
		assert.True(t, Exists(filepath.Join(dstDir, "subdir")))
		assert.True(t, Exists(filepath.Join(dstDir, "subdir", "file2.txt")))

		content1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
		assert.NoError(t, err)
		assert.Equal(t, []byte("file1"), content1)

		content2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
		assert.NoError(t, err)
		assert.Equal(t, []byte("file2"), content2)
	})

	t.Run("non-existent source", func(t *testing.T) {
		nonExistentDir := filepath.Join(tmpDir, "nonexistent")
		err := CopyDir(nonExistentDir, dstDir)
		assert.Error(t, err)
	})
}

func TestListDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "listdir_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	file1 := filepath.Join(tmpDir, "file1.txt")
	err = os.WriteFile(file1, []byte("file1"), 0644)
	assert.NoError(t, err)

	subDir := filepath.Join(tmpDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	assert.NoError(t, err)

	file2 := filepath.Join(subDir, "file2.txt")
	err = os.WriteFile(file2, []byte("file2"), 0644)
	assert.NoError(t, err)

	dsStore := filepath.Join(tmpDir, ".DS_Store")
	err = os.WriteFile(dsStore, []byte("dsstore"), 0644)
	assert.NoError(t, err)

	files, err := ListDir(tmpDir)
	assert.NoError(t, err)

	assert.Equal(t, 2, len(files))

	fileSet := make(map[string]bool)
	for _, file := range files {
		fileSet[file] = true
	}

	assert.True(t, fileSet[file1])
	assert.True(t, fileSet[file2])
	assert.False(t, fileSet[dsStore])

	_, err = ListDir(filepath.Join(tmpDir, "nonexistent"))
	assert.Error(t, err)
}

func TestReadFileLines(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "readlines_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5"
	err = os.WriteFile(tmpFile, []byte(content), 0644)
	assert.NoError(t, err)

	tests := []struct {
		name      string
		startLine int
		endLine   int
		expected  []string
	}{
		{"read all lines", 0, -1, []string{"line1", "line2", "line3", "line4", "line5"}},
		{"read first line", 0, 0, []string{"line1"}},
		{"read middle lines", 1, 3, []string{"line2", "line3", "line4"}},
		{"read to end", 2, -1, []string{"line3", "line4", "line5"}},
		{"start beyond file end", 10, -1, nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines, err := ReadFileLines(tmpFile, test.startLine, test.endLine)
			assert.NoError(t, err)
			assert.Equal(t, test.expected, lines)
		})
	}

	t.Run("non-existent file", func(t *testing.T) {
		_, err := ReadFileLines(filepath.Join(tmpDir, "nonexistent.txt"), 0, -1)
		assert.Error(t, err)
	})
}

func TestGetRelativePath(t *testing.T) {
	tests := []struct {
		name         string
		basePath     string
		absolutePath string
		expected     string
	}{
		{"same directory", "/base/dir", "/base/dir/file.txt", "file.txt"},
		{"subdirectory", "/base/dir", "/base/dir/sub/file.txt", "sub/file.txt"},
		{"parent directory", "/base/dir/sub", "/base/dir/file.txt", "../file.txt"},
		{"different drive windows", "C:\\base\\dir", "D:\\other\\file.txt", "../D:\\other\\file.txt"},
		{"invalid base", "invalid", "/base/dir/file.txt", "/base/dir/file.txt"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := GetRelativePath(test.basePath, test.absolutePath)
			expected := filepath.ToSlash(test.expected)
			assert.Equal(t, expected, result)
		})
	}
}
