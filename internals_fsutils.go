package seelog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type cannotOpenFileError struct {
	baseError
}

func newCannotOpenFileError(fname string) *cannotOpenFileError {
	return &cannotOpenFileError{baseError{message: "Cannot open file: " + fname}}
}

type notDirectoryError struct {
	baseError
}

func newNotDirectoryError(dname string) *notDirectoryError {
	return &notDirectoryError{baseError{message: dname + " is not directory"}}
}

// fileFilter is a filtering creterion function for '*os.File'.
// Must return 'false' to set aside the given file.
type fileFilter func(os.FileInfo, *os.File) bool

// filePathFilter is a filtering creterion function for a file path.
// Must return 'false' to set aside the given file.
type filePathFilter func(filePath string) bool

// GetSubdirNames returns a list of directories found in
// the given one with dirPath.
func getSubdirNames(dirPath string) ([]string, error) {
	fi, err := os.Stat(dirPath)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, newNotDirectoryError(dirPath)
	}
	dd, err := os.Open(dirPath)
	// Cannot open file.
	if err != nil {
		if dd != nil {
			dd.Close()
		}
		return nil, err
	}
	defer dd.Close()
	// TODO: Improve performance by buffering reading.
	allEntities, err := dd.Readdir(-1)
	if err != nil {
		return nil, err
	}
	subDirs := []string{}
	for _, entity := range allEntities {
		if entity.IsDir() {
			subDirs = append(subDirs, entity.Name())
		}
	}
	return subDirs, nil
}

// GetSubdirAbsPaths recursively visit all the subdirectories
// starting from the given directory and returns absolute paths for them.
func getAllSubdirAbsPaths(dirPath string) (res []string, err error) {
	dps, err := getSubdirAbsPaths(dirPath)
	if err != nil {
		res = []string{}
		return
	}
	res = append(res, dps...)
	for _, dp := range dps {
		sdps, err := getAllSubdirAbsPaths(dp)
		if err != nil {
			// res = []string{}
			return []string{}, err
		}
		res = append(res, sdps...)
	}
	return
}

// GetSubdirAbsPaths supplies absolute paths for all subdirectiries in a given directory.
// Input: (I1) dirPath - absolute path of a directory in question.
// Out: (O1) - slice of subdir asbolute paths; (O2) - error of the operation.
// Remark: If error (O2) is non-nil then (O1) is nil and vice versa.
func getSubdirAbsPaths(dirPath string) ([]string, error) {
	sdns, err := getSubdirNames(dirPath)
	if err != nil {
		return nil, err
	}
	rsdns := []string{}
	for _, sdn := range sdns {
		rsdns = append(rsdns, filepath.Join(dirPath, sdn))
	}
	return rsdns, nil
}

// GetOpenFilesInDir supplies a slice of os.File pointers to files located in the directory.
// Input: ...
// Output: ...
// Remark: Ignores files for which fileFilter returns false
func getOpenFilesInDir(dirPath string, fFilter fileFilter) ([]*os.File, error) {
	dfi, err := os.Open(dirPath)
	if err != nil {
		return nil, newCannotOpenFileError("Cannot open directory " + dirPath)
	}
	defer dfi.Close()
	// Size of read buffer (i.e. chunk of items read at a time).
	rbs := 64
	resFiles := []*os.File{}
L:
	for {
		// Read directory entities by reasonable chuncks
		// to prevent overflows on big number of files.
		fis, e := dfi.Readdir(rbs)
		switch e {
		// It's OK.
		case nil:
		// Do nothing, just continue cycle.
		case io.EOF:
			break L
		// Something went wrong.
		default:
			return nil, e
		}
		// THINK: Maybe, use async running.
		for _, fi := range fis {
			// NB: On Linux this could be a problem as
			// there are lots of file types available.
			if !fi.IsDir() {
				f, e := os.Open(filepath.Join(dirPath, fi.Name()))
				if e != nil {
					if f != nil {
						f.Close()
					}
					// THINK: Add nil as indicator that a problem occurred.
					resFiles = append(resFiles, nil)
					continue
				}
				// Check filter condition.
				if fFilter != nil && !fFilter(fi, f) {
					continue
				}
				resFiles = append(resFiles, f)
			}
		}
	}
	return resFiles, nil
}

// getDirFilePaths return full paths of the files located in the directory.
// Remark: Ignores files for which fileFilter returns false.
func getDirFilePaths(dirPath string, fpFilter filePathFilter) ([]string, error) {
	dfi, err := os.Open(dirPath)
	if err != nil {
		return nil, newCannotOpenFileError("Cannot open directory " + dirPath)
	}
	defer dfi.Close()
	// THINK: Maybe check if dirPath is really directory.

	// Size of read buffer (i.e. chunk of items read at a time).
	rbs := 2 << 5
	filePaths := []string{}

	var fp string
L:
	for {
		// Read directory entities by reasonable chuncks
		// to prevent overflows on big number of files.
		fis, e := dfi.Readdir(rbs)
		switch e {
		// It's OK.
		case nil:
		// Do nothing, just continue cycle.
		case io.EOF:
			break L
		// Indicate that something went wrong.
		default:
			return nil, e
		}
		// THINK: Maybe, use async running.
		for _, fi := range fis {
			// NB: Should work on every Windows and non-Windows OS.
			if fi.Mode().IsRegular() {
				// Build full path of a file.
				fp = filepath.Join(dirPath, fi.Name())
				// Check filter condition.
				if fpFilter != nil && !fpFilter(fp) {
					continue
				}
				filePaths = append(filePaths, fp)
			}
		}
	}
	return filePaths, nil
}

const (
	// Max number of directories can be read asynchronously.
	maxDirNumberReadAsync = 1e3
)

// getOpenFilesByDirectoryAsync runs async reading directories 'dirPaths' and inserts pairs
// in map 'filesInDirMap': Key - directory name, value - *os.File slice.
func getOpenFilesByDirectoryAsync(
	dirPaths []string,
	fFilter fileFilter,
	filesInDirMap map[string][]*os.File,
) error {
	n := len(dirPaths)
	if n > maxDirNumberReadAsync {
		return fmt.Errorf("Number of input directories to be read exceeded max value %d", maxDirNumberReadAsync)
	}
	type filesInDirResult struct {
		DirName string
		Files   []*os.File
		Error   error
	}
	dirFilesChan := make(chan *filesInDirResult, n)
	var wg sync.WaitGroup
	// Register n goroutines which are going to do work.
	wg.Add(n)
	for i := 0; i < n; i++ {
		// Launch asynchronously the piece of work.
		go func(dirPath string) {
			fs, e := getOpenFilesInDir(dirPath, fFilter)
			dirFilesChan <- &filesInDirResult{filepath.Base(dirPath), fs, e}
			// Mark the current goroutine as finished (work is done).
			wg.Done()
		}(dirPaths[i])
	}
	// Wait for all goroutines to finish their work.
	wg.Wait()
	// Close the error channel to let for-range clause
	// get all the buffered values without blocking and quit in the end.
	close(dirFilesChan)
	for fidr := range dirFilesChan {
		if fidr.Error == nil {
			// THINK: What will happen if the key is already present?
			filesInDirMap[fidr.DirName] = fidr.Files
		} else {
			return fidr.Error
		}
	}
	return nil
}

func copyFile(sf *os.File, dst string) (int64, error) {
	df, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer df.Close()
	return io.Copy(df, sf)
}

// fileExists return flag whether a given file exists
// and operation error if an unclassified failure occurs.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, err
		}
	}
	return true, nil
}

// createDirectory makes directory with a given name
// making all parent directories if necessary.
func createDirectory(dirPath string) error {
	var dPath string
	var err error
	if !filepath.IsAbs(dirPath) {
		dPath, err = filepath.Abs(dirPath)
		if err != nil {
			return err
		}
	} else {
		dPath = dirPath
	}
	exists, err := fileExists(dPath)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return os.MkdirAll(dPath, os.ModeDir)
}

func tryRemoveFile(filePath string) (err error) {
	err = os.Remove(filePath)
	if os.IsNotExist(err) {
		err = nil
		return
	}
	return
}