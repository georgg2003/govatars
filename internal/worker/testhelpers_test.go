package worker

import (
	"govatars/internal/pkg/config"
)

// minPNG is a 1×1 PNG decodable by the imaging library; used in worker delivery tests.
var minPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

// testRabbit returns a [config.RabbitMQ] suitable for unit tests of [worker.Processor].
//
//nolint:gosec // G101: RabbitMQ URL is test-only (guest credentials on loopback).
func testRabbit() config.RabbitMQ {
	return config.RabbitMQ{
		URL:                 "amqp://guest:guest@127.0.0.1:5672/",
		Exchange:            "govatars",
		UploadQueue:         "upload.q",
		UploadDLQRoutingKey: "upload.dlq",
		UploadRetryDelaysMS: []int{1000, 5000},
		DeleteQueue:         "delete.q",
		DeleteDLQRoutingKey: "delete.dlq",
		DeleteRetryDelaysMS: []int{1000},
	}
}

// testThumbCatalog builds the default thumbnail catalog from a normalized empty config.
func testThumbCatalog() config.ThumbnailCatalog {
	c := &config.App{}
	c.Normalize()
	cat, err := c.Avatars.Catalog()
	if err != nil {
		panic(err)
	}
	return cat
}
