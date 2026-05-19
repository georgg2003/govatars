//go:build integration

package rabbitmq_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/suite"
	tc "github.com/testcontainers/testcontainers-go"
	tcrabbit "github.com/testcontainers/testcontainers-go/modules/rabbitmq"

	"govatars/internal/pkg/config"
	rabbitrepo "govatars/internal/repository/rabbitmq"
)

// RabbitMQSuite shares one broker container across publisher / topology / DLQ tests.
// Each test uses its own per-test cfg with a unique suffix so queues don't collide.
type RabbitMQSuite struct {
	suite.Suite

	ctr     *tcrabbit.RabbitMQContainer
	amqpURL string
}

func (s *RabbitMQSuite) SetupSuite() {
	ctx := context.Background()
	ctr, err := tcrabbit.Run(ctx, "rabbitmq:3.12-management-alpine")
	s.Require().NoError(err)
	s.ctr = ctr

	s.amqpURL, err = ctr.AmqpURL(ctx)
	s.Require().NoError(err)
}

func (s *RabbitMQSuite) TearDownSuite() {
	if s.ctr != nil {
		if err := tc.TerminateContainer(s.ctr); err != nil {
			s.T().Logf("terminate rabbitmq container: %v", err)
		}
	}
}

// newConfig builds a topology config with a fresh suffix so a test never sees another test's queues.
// retryDelayMS controls how fast TTL-retry queues dead-letter back to the main queue.
func (s *RabbitMQSuite) newConfig(retryDelayMS int) config.RabbitMQ {
	suffix := strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	return config.RabbitMQ{
		URL:                 s.amqpURL,
		Exchange:            "it_ex_" + suffix,
		UploadRoutingKey:    "avatar.uploaded",
		DeleteRoutingKey:    "avatar.deleted",
		UploadQueue:         "it_upload_" + suffix,
		DeleteQueue:         "it_delete_" + suffix,
		UploadDLQQueue:      "it_upload_dlq_" + suffix,
		UploadDLQRoutingKey: "avatar.upload.failed",
		DeleteDLQQueue:      "it_delete_dlq_" + suffix,
		DeleteDLQRoutingKey: "avatar.delete.failed",
		UploadRetryDelaysMS: []int{retryDelayMS},
		DeleteRetryDelaysMS: []int{retryDelayMS},
	}
}

// newPublisher dials, declares topology, and registers cleanup; returns the publisher with that cfg.
func (s *RabbitMQSuite) newPublisher(cfg config.RabbitMQ) *rabbitrepo.Publisher {
	pub, err := rabbitrepo.NewPublisher(context.Background(), slog.New(slog.DiscardHandler), cfg)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		if cerr := pub.Close(context.Background()); cerr != nil {
			s.T().Logf("publisher close: %v", cerr)
		}
	})
	return pub
}

// newRawChannel opens a raw AMQP connection+channel for direct queue interactions.
// Both are torn down via t.Cleanup so the test only needs the channel.
func (s *RabbitMQSuite) newRawChannel() *amqp.Channel {
	conn, err := amqp.Dial(s.amqpURL)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		if cerr := conn.Close(); cerr != nil {
			s.T().Logf("amqp conn close: %v", cerr)
		}
	})
	ch, err := conn.Channel()
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		if cerr := ch.Close(); cerr != nil {
			s.T().Logf("amqp channel close: %v", cerr)
		}
	})
	return ch
}

func (s *RabbitMQSuite) TestDial_OK() {
	conn, err := rabbitrepo.Dial(s.amqpURL)
	s.Require().NoError(err)
	s.Require().NoError(conn.Close())
}

func (s *RabbitMQSuite) TestDial_EmptyURL() {
	_, err := rabbitrepo.Dial("")
	s.Require().Error(err)
}

func (s *RabbitMQSuite) TestPublishJSON_PersistentDelivery() {
	cfg := s.newConfig(100)
	pub := s.newPublisher(cfg)
	s.Require().NoError(pub.Health(context.Background()))

	ch := s.newRawChannel()
	consumerTag := "test-consumer-" + strings.TrimPrefix(cfg.UploadQueue, "it_upload_")
	msgs, err := ch.Consume(cfg.UploadQueue, consumerTag, true, false, false, false, nil)
	s.Require().NoError(err)

	body := map[string]string{"hello": "rabbit"}
	s.Require().NoError(pub.PublishJSON(context.Background(), cfg.UploadRoutingKey, "msg-1", body))

	select {
	case d := <-msgs:
		var got map[string]string
		s.Require().NoError(json.Unmarshal(d.Body, &got))
		s.Equal("rabbit", got["hello"])
		s.Equal("msg-1", d.MessageId)
	case <-time.After(15 * time.Second):
		s.FailNow("timed out waiting for message on upload queue")
	}
}

// TestDeclareTopology_RetryToMain proves that a message published to *.retry.0 dead-letters back
// to the main upload queue after the configured TTL via the DLX/DLRK declared by DeclareTopology.
func (s *RabbitMQSuite) TestDeclareTopology_RetryToMain() {
	cfg := s.newConfig(150) // small TTL so the test resolves quickly
	pub := s.newPublisher(cfg)
	_ = pub

	ch := s.newRawChannel()
	consumerTag := "retry-consumer-" + strings.TrimPrefix(cfg.UploadQueue, "it_upload_")
	msgs, err := ch.Consume(cfg.UploadQueue, consumerTag, true, false, false, false, nil)
	s.Require().NoError(err)

	retryQueue := cfg.UploadQueue + ".retry.0"
	body := []byte(`{"avatar_id":"retry","s3_key":"k"}`)
	s.Require().NoError(ch.PublishWithContext(context.Background(), "", retryQueue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
		MessageId:    "retry-1",
	}))

	select {
	case d := <-msgs:
		s.Equal(body, d.Body)
		s.Equal("retry-1", d.MessageId)
	case <-time.After(10 * time.Second):
		s.FailNow("timed out waiting for retry queue TTL to dead-letter back to main")
	}
}

// TestDeclareTopology_DLQRoutingKeyBindsToDLQ ensures DLQ binding works: messages published with
// the upload DLQ routing key on the topology exchange end up on the upload DLQ queue.
func (s *RabbitMQSuite) TestDeclareTopology_DLQRoutingKeyBindsToDLQ() {
	cfg := s.newConfig(100)
	pub := s.newPublisher(cfg)
	_ = pub

	ch := s.newRawChannel()
	consumerTag := "dlq-consumer-" + strings.TrimPrefix(cfg.UploadDLQQueue, "it_upload_dlq_")
	msgs, err := ch.Consume(cfg.UploadDLQQueue, consumerTag, true, false, false, false, nil)
	s.Require().NoError(err)

	body := []byte(`{"avatar_id":"poison"}`)
	s.Require().NoError(ch.PublishWithContext(context.Background(), cfg.Exchange, cfg.UploadDLQRoutingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
		MessageId:    "dlq-1",
	}))

	select {
	case d := <-msgs:
		s.Equal(body, d.Body)
		s.Equal("dlq-1", d.MessageId)
	case <-time.After(10 * time.Second):
		s.FailNow("timed out waiting for DLQ message")
	}
}

func TestRabbitMQSuite(t *testing.T) {
	suite.Run(t, new(RabbitMQSuite))
}
