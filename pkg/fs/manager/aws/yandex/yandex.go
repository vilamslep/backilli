package yandex

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/vilamslep/backilli/pkg/fs"
	env "github.com/vilamslep/backilli/pkg/fs/environment"
	"github.com/vilamslep/backilli/pkg/fs/unit"
)

var ErrLoadingConfiguration = fmt.Errorf("failed to load cloud configuration")

var limit int64 = 536870912

type YandexClient struct {
	s3client   *s3.Client
	bucketName string
	cloudSep   string
	cloudRoot  string
}

func NewClient(conf unit.ClientConfig) (*YandexClient, error) {

	env.Set("AWS_REGION", conf.Region)
	env.Set("AWS_ACCESS_KEY_ID", conf.KeyId)
	env.Set("AWS_SECRET_ACCESS_KEY", conf.KeySecret)

	customResolver := aws.EndpointResolverWithOptionsFunc(yandexResolver)

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithEndpointResolverWithOptions(customResolver))
	if err != nil {
		return nil, ErrLoadingConfiguration
	}

	s3client := s3.NewFromConfig(cfg)

	return &YandexClient{
		s3client:   s3client,
		cloudRoot:  conf.Root,
		cloudSep:   "/",
		bucketName: conf.BucketName,
	}, nil
}

func (c YandexClient) Read(path string) ([]byte, error) {
	object := &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(path),
	}
	resp, err := c.s3client.GetObject(context.Background(), object)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, int(resp.ContentLength))
	defer resp.Body.Close()
	var buffer bytes.Buffer
	for true {
		num, rerr := resp.Body.Read(buf)
		if num > 0 {
			buffer.Write(buf[:num])
		} else if rerr == io.EOF || rerr != nil {
			break
		}
	}
	return buffer.Bytes(), nil
}

func (c YandexClient) Write(src string, dst string) error {
	stat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if stat.Size()/limit > 0 {
		return c.putSplitedFile(src, dst)
	} else {
		return c.putOnce(src, dst)
	}
}

func (c YandexClient) putOnce(src string, dst string) error {
	return c.put(src, dst)
}

func (c YandexClient) putSplitedFile(src string, dst string) error {
	buf := make([]byte, limit)
	fd, err := os.Open(src)
	if err != nil {
		return err
	}
	defer fd.Close()

	stopped := false
	for i := 1; !stopped; i++ {
		stopped, err = c.writeAndPutPartOfFiles(fd, buf, i, dst)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c YandexClient) writeAndPutPartOfFiles(fd *os.File, buf []byte, part int, dst string) (bool, error) {
	if n, err := fd.Read(buf); err != nil {
		if err == io.EOF {
			return true, nil
		} else {
			return true, err
		}
	} else if n == 0 {
		return true, nil
	}
	//write temp file
	var err error

	fp := fs.GetFullPath("", os.TempDir(), fmt.Sprintf("zip.%03d", part))
	if fd, err = os.Create(fp); err == nil {
		if _, err := fd.Write(buf); err != nil {
			return true, err
		} else if err := fd.Close(); err != nil {
			return true, err
		}
	} else {
		return true, err
	}
	//put file to bucket
	if err := c.put(fp, fs.GetFullPath("", dst, fs.Base(fp))); err != nil {
		return false, err
	}
	//delete temp file
	if err := os.Remove(fp); err != nil {
		return true, err
	}
	return false, err
}

func (c YandexClient) put(src string, dst string) error {
	fd, err := os.Open(src)
	if err != nil {
		return err
	}
	defer fd.Close()

	stat, err := fd.Stat()
	if err != nil {
		return err
	}

	cloudRoot := c.cloudRoot
	if cloudRoot[len(cloudRoot)-1] == 0x5c ||
		cloudRoot[len(cloudRoot)-1] == 0x2f {
		cloudRoot = cloudRoot[:len(cloudRoot)-1]
	}

	s := bytes.ReplaceAll([]byte(dst), []byte{0x5c}, []byte{0x2f})

	yapath := fmt.Sprintf("%s%s%s", cloudRoot, c.cloudSep, string(s))

	object := &s3.PutObjectInput{
		Bucket:        aws.String(c.bucketName),
		Key:           aws.String(yapath),
		Body:          fd,
		ContentLength: stat.Size(),
	}

	if _, err = c.s3client.PutObject(context.Background(), object); err != nil {
		return err
	} else {
		return nil
	}
}

func (c YandexClient) Ls(path string) ([]unit.File, error) {
	var ls *s3.ListObjectsV2Output
	var err error

	params := &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucketName),
		Prefix: aws.String(path),
	}

	if ls, err = c.s3client.ListObjectsV2(context.TODO(), params); err != nil {
		return nil, err
	}

	files := make([]unit.File, 0, len(ls.Contents))
	for _, object := range ls.Contents {
		path := strings.Split(*object.Key, "/")

		name := path[len(path)-1]
		if name != "" {
			files = append(files, unit.File{
				Date: *object.LastModified,
				Name: name,
			})
		}
	}

	return files, nil
}

func (c YandexClient) Remove(path string) error {
	deleteParams := &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(path),
	}

	if _, err := c.s3client.DeleteObject(context.TODO(), deleteParams); err != nil {
		return err
	}
	return nil
}

func (c YandexClient) Close() error {
	return nil
}

func yandexResolver(service string, region string, options ...interface{}) (aws.Endpoint, error) {
	if service == s3.ServiceID && region == "ru-central1" {
		return aws.Endpoint{
			PartitionID:   "yc",
			URL:           "https://storage.yandexcloud.net",
			SigningRegion: "ru-central1",
		}, nil
	}
	return aws.Endpoint{}, fmt.Errorf("unknown endpoint requested")
}
