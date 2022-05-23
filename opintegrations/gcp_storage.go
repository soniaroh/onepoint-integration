package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"

	"cloud.google.com/go/storage"
)

// NOTE: this uses a service account, you must set a environment variable
// see https://cloud.google.com/storage/docs/reference/libraries

func uploadFileGCM(BUCKET_NAME, gcmFileName string, file *os.File) error {
	ctx := context.Background()

	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}

	bucket := client.Bucket(BUCKET_NAME)

	obj := bucket.Object(gcmFileName)

	w := obj.NewWriter(ctx)
	w.CacheControl = "no-cache"
	w.ACL = []storage.ACLRule{{Entity: storage.AllUsers, Role: storage.RoleReader}}

	r := bufio.NewReader(file)
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if err != nil && err != io.EOF {
			ErrorLog.Printf("%v\n", err)
			break
		}
		if n == 0 {
			break
		}

		if _, err := w.Write(buf[:n]); err != nil {
			ErrorLog.Printf("%v\n", err)
			break
		}
	}

	if err := w.Close(); err != nil {
		return err
	}

	return nil
}

func bytesToGCP(BUCKET_NAME, gcmFileName string, data []byte, setObjectPolicies bool) error {
	ctx := context.Background()

	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}

	bucket := client.Bucket(BUCKET_NAME)

	obj := bucket.Object(gcmFileName)
	w := obj.NewWriter(ctx)

	if setObjectPolicies {
		w.CacheControl = "no-cache"
		w.ACL = []storage.ACLRule{{Entity: storage.AllUsers, Role: storage.RoleReader}}
	}

	r := bytes.NewReader(data)
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if err != nil && err != io.EOF {
			ErrorLog.Printf("%v\n", err)
			break
		}
		if n == 0 {
			break
		}

		if _, err := w.Write(buf[:n]); err != nil {
			ErrorLog.Printf("%v\n", err)
			break
		}
	}

	if err := w.Close(); err != nil {
		return err
	}

	return nil
}
