package common

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingReader fails after emitting some bytes, simulating a broken upload.
type failingReader struct {
	emitted bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.emitted {
		return 0, errors.New("simulated upstream failure")
	}
	r.emitted = true
	n := copy(p, []byte("partial-payload"))
	return n, nil
}

// TestSaveTmpFileRemovesFileOnCopyFailure 锁定临时文件泄漏修复：
// 复制数据失败时不能把半成品文件留在临时目录里。
func TestSaveTmpFileRemovesFileOnCopyFailure(t *testing.T) {
	before := listTempEntries(t, "leak-check-*")

	filename, err := SaveTmpFile("leak-check-*", &failingReader{})

	require.Error(t, err)
	assert.Empty(t, filename, "failed save must not report a usable path")

	after := listTempEntries(t, "leak-check-*")
	assert.Equal(t, before, after, "no leftover temp file may remain after a failed copy")
}

// TestSaveTmpFileKeepsFileOnSuccess 确保成功路径行为不变，调用方仍可读取内容。
func TestSaveTmpFileKeepsFileOnSuccess(t *testing.T) {
	filename, err := SaveTmpFile("tmpfile-success-*", strings.NewReader("hello new-api"))
	require.NoError(t, err)
	require.NotEmpty(t, filename)
	t.Cleanup(func() { _ = os.Remove(filename) })

	content, err := os.ReadFile(filename)
	require.NoError(t, err)
	assert.Equal(t, "hello new-api", string(content))
}

func listTempEntries(t *testing.T, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), pattern))
	require.NoError(t, err)
	return matches
}
