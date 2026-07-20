package auditarchive

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint  *url.URL
	AccessKey string
	SecretKey string
	Region    string
}

type S3Store struct {
	client *minio.Client
}

func NewS3Store(cfg S3Config) (*S3Store, error) {
	client, err := minio.New(cfg.Endpoint.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.Endpoint.Scheme == "https",
		Region:       cfg.Region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, err
	}
	return &S3Store{client: client}, nil
}

func (store *S3Store) Stat(ctx context.Context, bucket, object string) (ObjectInfo, error) {
	info, err := store.client.StatObject(ctx, bucket, object, minio.StatObjectOptions{})
	if err != nil {
		response := minio.ToErrorResponse(err)
		if response.StatusCode == 404 || response.Code == "NoSuchKey" || response.Code == "NoSuchObject" {
			return ObjectInfo{}, ErrObjectNotFound
		}
		return ObjectInfo{}, err
	}
	metadata := make(map[string]string, len(info.UserMetadata)+len(info.Metadata))
	for key, value := range info.UserMetadata {
		metadata[key] = value
	}
	for key, values := range info.Metadata {
		if len(values) != 0 {
			metadata[key] = values[0]
		}
	}
	return ObjectInfo{Size: info.Size, Metadata: metadata}, nil
}

func (store *S3Store) Put(ctx context.Context, bucket, object string, reader io.Reader, size int64, options PutOptions) error {
	mode := minio.RetentionMode(strings.ToUpper(options.RetentionMode))
	if !mode.IsValid() {
		return errors.New("invalid S3 object retention mode")
	}
	_, err := store.client.PutObject(ctx, bucket, object, reader, size, minio.PutObjectOptions{
		ContentType:      options.ContentType,
		UserMetadata:     options.Metadata,
		Mode:             mode,
		RetainUntilDate:  options.RetentionUntil,
		DisableMultipart: true,
	})
	return err
}
