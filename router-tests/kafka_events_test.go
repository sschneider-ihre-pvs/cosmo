package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"github.com/hasura/go-graphql-client"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/wundergraph/cosmo/router-tests/testenv"
	"github.com/wundergraph/cosmo/router/pkg/config"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

func TestKafkaEvents(t *testing.T) {
	// All tests are running in sequence because they are using the same kafka topic

	if os.Getenv("TEST_KAFKA") == "" {
		t.Skip("Skipping test in CI environment")
	}

	ctx := context.Background()
	kafkaContainer, err := kafka.RunContainer(ctx, testcontainers.WithImage("confluentinc/confluent-local:7.6.1"))
	require.NoError(t, err)

	require.NoError(t, kafkaContainer.Start(ctx))

	seeds, err := kafkaContainer.Brokers(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, kafkaContainer.Terminate(ctx))
	})

	t.Run("subscribe async", func(t *testing.T) {

		topicName := "employeeUpdated"

		testenv.Run(t, &testenv.Config{
			KafkaSeeds: seeds,
		}, func(t *testing.T, xEnv *testenv.Environment) {

			ensureTopicExists(t, xEnv, topicName)

			var subscriptionOne struct {
				employeeUpdatedMyKafka struct {
					ID      float64 `graphql:"id"`
					Details struct {
						Forename string `graphql:"forename"`
						Surname  string `graphql:"surname"`
					} `graphql:"details"`
				} `graphql:"employeeUpdatedMyKafka(employeeID: 3)"`
			}

			surl := xEnv.GraphQLSubscriptionURL()
			client := graphql.NewSubscriptionClient(surl)
			t.Cleanup(func() {
				_ = client.Close()
			})

			wg := &sync.WaitGroup{}
			wg.Add(1)

			subscriptionOneID, err := client.Subscribe(&subscriptionOne, nil, func(dataValue []byte, errValue error) error {
				defer wg.Done()
				require.NoError(t, errValue)
				require.JSONEq(t, `{"employeeUpdatedMyKafka":{"id":1,"details":{"forename":"Jens","surname":"Neuse"}}}`, string(dataValue))
				return nil
			})
			require.NoError(t, err)
			require.NotEmpty(t, subscriptionOneID)

			go func() {
				clientErr := client.Run()
				require.NoError(t, clientErr)
			}()

			go func() {
				wg.Wait()
				unsubscribeErr := client.Unsubscribe(subscriptionOneID)
				require.NoError(t, unsubscribeErr)
				clientCloseErr := client.Close()
				require.NoError(t, clientCloseErr)
			}()

			xEnv.WaitForSubscriptionCount(1, time.Second*10)

			produceKafkaMessage(t, xEnv, topicName, `{"__typename":"Employee","id": 1,"update":{"name":"foo"}}`)

			xEnv.WaitForMessagesSent(1, time.Second*10)
			xEnv.WaitForSubscriptionCount(0, time.Second*10)
			xEnv.WaitForConnectionCount(0, time.Second*10)
		})
	})

	t.Run("message and resolve errors should not abort the subscription", func(t *testing.T) {

		topicName := "employeeUpdated"

		testenv.Run(t, &testenv.Config{
			KafkaSeeds: seeds,
		}, func(t *testing.T, xEnv *testenv.Environment) {

			ensureTopicExists(t, xEnv, topicName)

			var subscriptionOne struct {
				employeeUpdatedMyKafka struct {
					ID      float64 `graphql:"id"`
					Details struct {
						Forename string `graphql:"forename"`
						Surname  string `graphql:"surname"`
					} `graphql:"details"`
				} `graphql:"employeeUpdatedMyKafka(employeeID: 3)"`
			}

			surl := xEnv.GraphQLSubscriptionURL()
			client := graphql.NewSubscriptionClient(surl)
			t.Cleanup(func() {
				_ = client.Close()
			})

			wg := &sync.WaitGroup{}
			wg.Add(4)

			count := 0

			subscriptionOneID, err := client.Subscribe(&subscriptionOne, nil, func(dataValue []byte, errValue error) error {
				defer wg.Done()

				if count == 0 {
					var gqlErr graphql.Errors
					require.ErrorAs(t, errValue, &gqlErr)
					require.Equal(t, "Internal server error", gqlErr[0].Message)
				} else if count == 1 || count == 3 {
					require.NoError(t, errValue)
					require.JSONEq(t, `{"employeeUpdatedMyKafka":{"id":1,"details":{"forename":"Jens","surname":"Neuse"}}}`, string(dataValue))
				} else if count == 2 {
					var gqlErr graphql.Errors
					require.ErrorAs(t, errValue, &gqlErr)
					require.Equal(t, "Cannot return null for non-nullable field 'Subscription.employeeUpdatedMyKafka.id'.", gqlErr[0].Message)
				}

				count++

				return nil
			})
			require.NoError(t, err)
			require.NotEmpty(t, subscriptionOneID)

			go func() {
				clientErr := client.Run()
				require.NoError(t, clientErr)
			}()

			xEnv.WaitForSubscriptionCount(1, time.Second*10)

			produceKafkaMessage(t, xEnv, topicName, ``) // Empty message
			xEnv.WaitForMessagesSent(1, time.Second*10)

			produceKafkaMessage(t, xEnv, topicName, `{"__typename":"Employee","id": 1,"update":{"name":"foo"}}`) // Correct message
			xEnv.WaitForMessagesSent(2, time.Second*10)

			produceKafkaMessage(t, xEnv, topicName, `{"__typename":"Employee","update":{"name":"foo"}}`) // Missing entity = Resolver error
			xEnv.WaitForMessagesSent(3, time.Second*10)

			produceKafkaMessage(t, xEnv, topicName, `{"__typename":"Employee","id": 1,"update":{"name":"foo"}}`) // Correct message
			xEnv.WaitForMessagesSent(4, time.Second*10)

			wg.Wait()

			unsubscribeErr := client.Unsubscribe(subscriptionOneID)
			require.NoError(t, unsubscribeErr)

			clientCloseErr := client.Close()
			require.NoError(t, clientCloseErr)

			xEnv.WaitForSubscriptionCount(0, time.Second*10)
			xEnv.WaitForConnectionCount(0, time.Second*10)
		})
	})

	t.Run("every subscriber gets the message", func(t *testing.T) {

		topicName := "employeeUpdated"

		testenv.Run(t, &testenv.Config{
			KafkaSeeds: seeds,
		}, func(t *testing.T, xEnv *testenv.Environment) {

			ensureTopicExists(t, xEnv, topicName)

			var subscriptionOne struct {
				employeeUpdatedMyKafka struct {
					ID      float64 `graphql:"id"`
					Details struct {
						Forename string `graphql:"forename"`
						Surname  string `graphql:"surname"`
					} `graphql:"details"`
				} `graphql:"employeeUpdatedMyKafka(employeeID: 3)"`
			}

			surl := xEnv.GraphQLSubscriptionURL()
			client := graphql.NewSubscriptionClient(surl)
			t.Cleanup(func() {
				_ = client.Close()
			})

			wg := &sync.WaitGroup{}
			wg.Add(2)

			subscriptionOneID, err := client.Subscribe(&subscriptionOne, nil, func(dataValue []byte, errValue error) error {
				defer wg.Done()
				require.NoError(t, errValue)
				require.JSONEq(t, `{"employeeUpdatedMyKafka":{"id":1,"details":{"forename":"Jens","surname":"Neuse"}}}`, string(dataValue))
				return nil
			})
			require.NoError(t, err)
			require.NotEmpty(t, subscriptionOneID)

			subscriptionTwoID, err := client.Subscribe(&subscriptionOne, nil, func(dataValue []byte, errValue error) error {
				defer wg.Done()
				require.NoError(t, errValue)
				require.JSONEq(t, `{"employeeUpdatedMyKafka":{"id":1,"details":{"forename":"Jens","surname":"Neuse"}}}`, string(dataValue))
				return nil
			})
			require.NoError(t, err)
			require.NotEmpty(t, subscriptionTwoID)

			go func() {
				clientErr := client.Run()
				require.NoError(t, clientErr)
			}()

			go func() {
				wg.Wait()
				unsubscribeErr := client.Unsubscribe(subscriptionOneID)
				require.NoError(t, unsubscribeErr)

				unsubscribeErr = client.Unsubscribe(subscriptionTwoID)
				require.NoError(t, unsubscribeErr)

				clientCloseErr := client.Close()
				require.NoError(t, clientCloseErr)
			}()

			xEnv.WaitForSubscriptionCount(1, time.Second*10)

			produceKafkaMessage(t, xEnv, topicName, `{"__typename":"Employee","id": 1,"update":{"name":"foo"}}`)

			xEnv.WaitForMessagesSent(2, time.Second*10)
			xEnv.WaitForSubscriptionCount(0, time.Second*10)
			xEnv.WaitForConnectionCount(0, time.Second*10)
		})
	})

	t.Run("subscribe async epoll/kqueue disabled", func(t *testing.T) {

		topicName := "employeeUpdated"

		testenv.Run(t, &testenv.Config{
			KafkaSeeds: seeds,
			ModifyEngineExecutionConfiguration: func(engineExecutionConfiguration *config.EngineExecutionConfiguration) {
				engineExecutionConfiguration.EnableWebSocketEpollKqueue = false
				engineExecutionConfiguration.WebSocketReadTimeout = time.Millisecond * 100
			},
		}, func(t *testing.T, xEnv *testenv.Environment) {

			ensureTopicExists(t, xEnv, topicName)

			var subscriptionOne struct {
				employeeUpdatedMyKafka struct {
					ID      float64 `graphql:"id"`
					Details struct {
						Forename string `graphql:"forename"`
						Surname  string `graphql:"surname"`
					} `graphql:"details"`
				} `graphql:"employeeUpdatedMyKafka(employeeID: 3)"`
			}

			surl := xEnv.GraphQLSubscriptionURL()
			client := graphql.NewSubscriptionClient(surl)
			t.Cleanup(func() {
				_ = client.Close()
			})

			wg := &sync.WaitGroup{}
			wg.Add(1)

			subscriptionOneID, err := client.Subscribe(&subscriptionOne, nil, func(dataValue []byte, errValue error) error {
				defer wg.Done()
				require.NoError(t, errValue)
				require.JSONEq(t, `{"employeeUpdatedMyKafka":{"id":1,"details":{"forename":"Jens","surname":"Neuse"}}}`, string(dataValue))
				return nil
			})
			require.NoError(t, err)
			require.NotEmpty(t, subscriptionOneID)

			go func() {
				clientErr := client.Run()
				require.NoError(t, clientErr)
			}()

			go func() {
				wg.Wait()
				unsubscribeErr := client.Unsubscribe(subscriptionOneID)
				require.NoError(t, unsubscribeErr)
				clientCloseErr := client.Close()
				require.NoError(t, clientCloseErr)
			}()

			xEnv.WaitForSubscriptionCount(1, time.Second*10)

			produceKafkaMessage(t, xEnv, topicName, `{"__typename":"Employee","id": 1,"update":{"name":"foo"}}`)

			xEnv.WaitForMessagesSent(1, time.Second*10)
			xEnv.WaitForSubscriptionCount(0, time.Second*10)
			xEnv.WaitForConnectionCount(0, time.Second*10)
		})
	})

	t.Run("subscribe sync sse", func(t *testing.T) {

		topicName := "employeeUpdated"

		testenv.Run(t, &testenv.Config{
			KafkaSeeds: seeds,
		}, func(t *testing.T, xEnv *testenv.Environment) {

			ensureTopicExists(t, xEnv, topicName)

			subscribePayload := []byte(`{"query":"subscription { employeeUpdatedMyKafka(employeeID: 1) { id details { forename surname } }}"}`)

			wg := &sync.WaitGroup{}
			wg.Add(1)

			go func() {
				client := http.Client{
					Timeout: time.Second * 10,
				}
				req, err := http.NewRequest(http.MethodPost, xEnv.GraphQLServeSentEventsURL(), bytes.NewReader(subscribePayload))
				require.NoError(t, err)

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "text/event-stream")
				req.Header.Set("Connection", "keep-alive")
				req.Header.Set("Cache-Control", "no-cache")

				resp, err := client.Do(req)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode)
				defer resp.Body.Close()
				reader := bufio.NewReader(resp.Body)

				eventNext, _, err := reader.ReadLine()
				require.NoError(t, err)
				require.Equal(t, "event: next", string(eventNext))
				data, _, err := reader.ReadLine()
				require.NoError(t, err)
				require.Equal(t, "data: {\"data\":{\"employeeUpdatedMyKafka\":{\"id\":1,\"details\":{\"forename\":\"Jens\",\"surname\":\"Neuse\"}}}}", string(data))
				line, _, err := reader.ReadLine()
				require.NoError(t, err)
				require.Equal(t, "", string(line))

				wg.Done()

			}()

			xEnv.WaitForSubscriptionCount(1, time.Second*5)

			produceKafkaMessage(t, xEnv, topicName, `{"__typename":"Employee","id": 1,"update":{"name":"foo"}}`)

			wg.Wait()

			xEnv.WaitForSubscriptionCount(0, time.Second*10)
			xEnv.WaitForConnectionCount(0, time.Second*10)
		})
	})

	t.Run("subscribe sync sse with block", func(t *testing.T) {
		t.Parallel()

		subscribePayload := []byte(`{"query":"subscription { employeeUpdatedMyKafka(employeeID: 1) { id details { forename surname } }}"}`)

		testenv.Run(t, &testenv.Config{
			KafkaSeeds: seeds,
			ModifySecurityConfiguration: func(securityConfiguration *config.SecurityConfiguration) {
				securityConfiguration.BlockSubscriptions = true
			},
		}, func(t *testing.T, xEnv *testenv.Environment) {
			client := http.Client{
				Timeout: time.Second * 10,
			}
			req, err := http.NewRequest(http.MethodPost, xEnv.GraphQLServeSentEventsURL(), bytes.NewReader(subscribePayload))
			require.NoError(t, err)

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "text/event-stream")
			req.Header.Set("Connection", "keep-alive")
			req.Header.Set("Cache-Control", "no-cache")

			resp, err := client.Do(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			defer resp.Body.Close()
			reader := bufio.NewReader(resp.Body)

			eventNext, _, err := reader.ReadLine()
			require.NoError(t, err)
			require.Equal(t, "event: next", string(eventNext))
			data, _, err := reader.ReadLine()
			require.NoError(t, err)
			require.Equal(t, "data: {\"errors\":[{\"message\":\"operation type 'subscription' is blocked\"}],\"data\":null}", string(data))

			xEnv.WaitForSubscriptionCount(0, time.Second*10)
			xEnv.WaitForConnectionCount(0, time.Second*10)
		})
	})
}

func ensureTopicExists(t *testing.T, xEnv *testenv.Environment, topicName string) {
	// Delete topic for idempotency

	deleteCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := xEnv.KafkaAdminClient.DeleteTopics(deleteCtx, topicName)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = xEnv.KafkaAdminClient.CreateTopic(ctx, 1, 1, nil, topicName)
	require.NoError(t, err)
}

func produceKafkaMessage(t *testing.T, xEnv *testenv.Environment, topicName string, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	var pErr error

	xEnv.KafkaClient.Produce(ctx, &kgo.Record{
		Topic: topicName,
		Value: []byte(message),
	}, func(record *kgo.Record, err error) {
		defer wg.Done()
		if err != nil {
			pErr = err
		}
	})

	wg.Wait()

	require.NoError(t, pErr)
}
