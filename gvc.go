package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/glide/cfg"
	gpath "github.com/Masterminds/glide/path"
	"github.com/spf13/cobra"
)

var cmd = &cobra.Command{
	Use:   "gvc",
	Short: "glide vendor cleaner",
	Run:   gvc,
}

type config struct {
	dryrun    bool
	keepAll   bool
	keepTests bool
}

var conf config

func init() {
	cmd.PersistentFlags().BoolVar(&conf.dryrun, "dryrun", false, "just output what will be removed")
	cmd.PersistentFlags().BoolVar(&conf.keepAll, "keep-all", false, "keep all files of needed packages (instead of keeping only not test .go files)")
	cmd.PersistentFlags().BoolVar(&conf.keepTests, "keep-tests", false, "keep also go test files")
}

func main() {
	cmd.Execute()
}

func gvc(cmd *cobra.Command, args []string) {
	lock, err := LoadGlideLockfile(".")
	if err != nil {
		fmt.Errorf("Could not load lockfile: %v", err)
		os.Exit(1)
	}

	pkgList := []string{}
	// TODO(sgotti) Should we also consider devImports?
	for _, imp := range lock.Imports {
		if len(imp.Subpackages) > 0 {
			for _, sp := range imp.Subpackages {
				pkgList = append(pkgList, filepath.Join(imp.Name, sp))
			}
		}
		// TODO(sgotti) we cannot skip the base import if it has subpackages
		// because glide doesn't write "." as a subpackage, otherwise if some
		// files in the base import are needed they will be removed.
		pkgList = append(pkgList, imp.Name)
	}

	if err := cleanup(pkgList); err != nil {
		fmt.Printf("cleanup error: %v", err)
		os.Exit(1)
	}
}

func cleanup(pkgList []string) error {
	vpath, err := gpath.Vendor()
	if err != nil {
		return err
	}
	if vpath == "" {
		return fmt.Errorf("cannot fine vendor dir")
	}

	var searchPath string
	var markForDelete []struct {
		path  string
		isDir bool
	}

	fn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == searchPath || path == vpath {
			return nil
		}

		localPath := strings.TrimPrefix(path, searchPath)
		keep := false

		// If the file's parent directory is a needed package, keep it.
		for _, name := range pkgList {
			if !info.IsDir() && filepath.Dir(localPath) == name {
				if conf.keepAll {
					keep = true
					continue
				} else if conf.keepTests {
					if strings.HasSuffix(path, ".go") {
						keep = true
						continue
					}
				} else if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
					keep = true
				}
			}
		}

		// If a directory is the needed package or a parent then keep it
		if keep == false && info.IsDir() {
			for _, name := range pkgList {
				if strings.HasPrefix(name, localPath) {
					keep = true
				}
			}
		}

		// Avoid marking for removal childs of already marked directories
		for _, marked := range markForDelete {
			if marked.isDir {
				if strings.HasPrefix(filepath.Dir(path), marked.path) {
					return nil
				}
			}
		}

		if keep == false {
			// Mark for deletion
			markForDelete = append(markForDelete, struct {
				path  string
				isDir bool
			}{path, info.IsDir()})
		}

		return nil
	}

	// Walk vendor directory
	searchPath = vpath + string(os.PathSeparator)
	err = filepath.Walk(searchPath, fn)
	if err != nil {
		return err
	}

	// Perform the actual delete.
	for _, marked := range markForDelete {
		localPath := strings.TrimPrefix(marked.path, searchPath)
		if marked.isDir {
			fmt.Printf("Removing unused dir: %s\n", localPath)
		} else {
			fmt.Printf("Removing unused file: %s\n", localPath)
		}
		if !conf.dryrun {
			rerr := os.RemoveAll(marked.path)
			if rerr != nil {
				return rerr
			}
		}
	}

	return nil
}

// LoadLockfile loads the contents of a glide.lock file.
func LoadGlideLockfile(base string) (*cfg.Lockfile, error) {
	yml, err := ioutil.ReadFile(filepath.Join(base, gpath.LockFile))
	if err != nil {
		return nil, err
	}
	lock, err := cfg.LockfileFromYaml(yml)
	if err != nil {
		return nil, err
	}

	return lock, nil
}
