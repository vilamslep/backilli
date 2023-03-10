package local

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/vilamslep/backilli/pkg/fs"
	"github.com/vilamslep/backilli/pkg/fs/unit"
)

type LocalClient struct {
	root string
}

func NewClient(conf unit.ClientConfig) LocalClient {
	return LocalClient{
		root: conf.Root,
	}
}

func (c LocalClient) Read(path string) ([]byte, error) {
	fd, err := os.OpenFile(path, os.O_RDWR, os.ModePerm)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	stat, err := fd.Stat()
	if err != nil {
		return nil, err
	}

	res := make([]byte, stat.Size())
	buffer := make([]byte, 2048)

	offs := 0
	for {
		n, err := fd.Read(buffer)

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}
		for i := 0; i < n; i++ {
			res[(offs + i)] = buffer[i]
		}
		offs += len(buffer)
	}
	return res, nil
}

func (c LocalClient) Write(src string, dst string) error {
	_, err := os.Stat(c.root)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(c.root, os.ModePerm); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	fpf := fs.GetFullPath("", c.root, dst)
	fpd := fs.Dir(fpf)
	_, err = os.Stat(fpd)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(fpd, os.ModePerm); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	_, err = os.Stat(fpf)
	if os.IsExist(err) {
		if err := os.RemoveAll(fpf);err != nil {
			return err
		}
	} 

	fd, err := os.OpenFile(fpf, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer fd.Close()

	rd, err := os.OpenFile(src, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer rd.Close()

	var bufferOffset int64 = 4096

	buf := make([]byte, bufferOffset)

	for {
		n, err := rd.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if n > 0 {
			if _, err := fd.Write(buf); err != nil {
				return err
			}
			continue
		}
	}

	return err
}

func (c LocalClient) Ls(path string) ([]unit.File, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("file is not a directory")
	}

	ls, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}
	res := make([]unit.File, len(ls))
	for i, f := range ls {
		res[i] = unit.File{
			Name: f.Name(),
			Date: f.ModTime(),
		}
	}

	return res, nil
}

func (c LocalClient) Remove(path string) error {
	return os.RemoveAll(path)
}

func (c LocalClient) Close() error {
	return nil
}
