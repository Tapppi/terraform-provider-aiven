package aiven

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/aiven/aiven-go-client"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func init() {
	resource.AddTestSweepers("aiven_kafka_topic", &resource.Sweeper{
		Name: "aiven_kafka_topic",
		F:    sweepKafkaTopics,
	})
}

func sweepKafkaTopics(region string) error {
	client, err := sharedClient(region)
	if err != nil {
		return fmt.Errorf("error getting client: %s", err)
	}

	conn := client.(*aiven.Client)

	projects, err := conn.Projects.List()
	if err != nil {
		return fmt.Errorf("error retrieving a list of projects : %s", err)
	}

	for _, project := range projects {
		if project.Name == os.Getenv("AIVEN_PROJECT_NAME") {
			services, err := conn.Services.List(project.Name)
			if err != nil {
				return fmt.Errorf("error retrieving a list of services for a project `%s`: %s", project.Name, err)
			}

			for _, service := range services {
				if service.Type != "kafka" {
					continue
				}

				topics, err := conn.KafkaTopics.List(project.Name, service.Name)
				if err != nil {
					log.Printf("[ERROR] error retrieving a list of kafka topics for a service `%s`: %s", service.Name, err)
					continue
				}

				for _, topic := range topics {
					err = conn.KafkaTopics.Delete(project.Name, service.Name, topic.TopicName)
					if err != nil {
						return fmt.Errorf("error destroying kafka topic %s during sweep: %s", topic.TopicName, err)
					}
				}
			}
		}
	}

	return nil
}

func TestAccAivenKafkaTopic_basic(t *testing.T) {
	resourceName := "aiven_kafka_topic.foo"
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	rName2 := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	rName3 := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	rName4 := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckAivenKafkaTopicResourceDestroy,
		Steps: []resource.TestStep{
			// basic Kafka Topic test
			{
				Config: testAccKafkaTopicResource(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAivenKafkaTopicAttributes("data.aiven_kafka_topic.topic"),
					resource.TestCheckResourceAttr(resourceName, "project", os.Getenv("AIVEN_PROJECT_NAME")),
					resource.TestCheckResourceAttr(resourceName, "service_name", fmt.Sprintf("test-acc-sr-%s", rName)),
					resource.TestCheckResourceAttr(resourceName, "topic_name", fmt.Sprintf("test-acc-topic-%s", rName)),
					resource.TestCheckResourceAttr(resourceName, "partitions", "3"),
					resource.TestCheckResourceAttr(resourceName, "replication", "2"),
					resource.TestCheckResourceAttr(resourceName, "termination_protection", "false"),
				),
			},
			// basic Kafka Topic test validating is already exists error handling
			{
				// Before running Config Kafka service and topic will be created via API
				PreConfig: func() {
					client, _ := aiven.NewTokenClient(os.Getenv("AIVEN_TOKEN"), "terraform-provider-aiven/")
					s, err := client.Services.Create(os.Getenv("AIVEN_PROJECT_NAME"), aiven.CreateServiceRequest{
						Cloud:       "google-europe-west1",
						Plan:        "business-4",
						ServiceName: fmt.Sprintf("test-acc-sr-%s", rName2),
						ServiceType: "kafka",
						MaintenanceWindow: &aiven.MaintenanceWindow{
							DayOfWeek: "monday",
							TimeOfDay: "10:00:00",
						},
					})
					if err != nil {
						t.Fatalf("Cannot create Kafka service in PreConfig %s:", err.Error())
					}

					// wait until Kafka service is RUNNING
					for {
						s, err = client.Services.Get(os.Getenv("AIVEN_PROJECT_NAME"), s.Name)
						if err != nil {
							t.Fatalf("Cannot get Kafka service in PreConfig while waiting to be ACTIVE %s:", err.Error())
						}

						if s.State == "RUNNING" {
							t.Logf("Service %s is RUNNING!", s.Name)
							time.Sleep(10 * time.Second)
							break
						}

						t.Logf("Waiting for service %s to be RUNNING, current state: %s", s.Name, s.State)
						time.Sleep(10 * time.Second)
					}

					var partitions = 3
					var replication = 2
					err = client.KafkaTopics.Create(os.Getenv("AIVEN_PROJECT_NAME"), s.Name, aiven.CreateKafkaTopicRequest{
						Partitions:  &partitions,
						Replication: &replication,
						TopicName:   fmt.Sprintf("test-acc-topic-%s", rName),
						Config: aiven.KafkaTopicConfig{
							FlushMs:                     parseOptionalStringToInt64("10"),
							UncleanLeaderElectionEnable: parseOptionalStringToBool("true"),
							CleanupPolicy:               "compact",
							MinCleanableDirtyRatio:      parseOptionalStringToFloat64("0.01"),
						},
					})
					if err != nil {
						t.Fatalf("Cannot create Kafka Topic in PreConfig %s:", err.Error())
					}
				},
				Config: testAccKafkaTopicResource(rName2),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAivenKafkaTopicAttributes("data.aiven_kafka_topic.topic"),
					resource.TestCheckResourceAttr(resourceName, "project", os.Getenv("AIVEN_PROJECT_NAME")),
					resource.TestCheckResourceAttr(resourceName, "service_name", fmt.Sprintf("test-acc-sr-%s", rName2)),
					resource.TestCheckResourceAttr(resourceName, "topic_name", fmt.Sprintf("test-acc-topic-%s", rName2)),
					resource.TestCheckResourceAttr(resourceName, "partitions", "3"),
					resource.TestCheckResourceAttr(resourceName, "replication", "2"),
					resource.TestCheckResourceAttr(resourceName, "termination_protection", "false"),
				),
			},
			// custom TF client timeouts test
			{
				Config: testAccKafkaTopicCustomTimeoutsResource(rName3),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAivenKafkaTopicAttributes("data.aiven_kafka_topic.topic"),
					resource.TestCheckResourceAttr(resourceName, "project", os.Getenv("AIVEN_PROJECT_NAME")),
					resource.TestCheckResourceAttr(resourceName, "service_name", fmt.Sprintf("test-acc-sr-%s", rName3)),
					resource.TestCheckResourceAttr(resourceName, "topic_name", fmt.Sprintf("test-acc-topic-%s", rName3)),
					resource.TestCheckResourceAttr(resourceName, "partitions", "3"),
					resource.TestCheckResourceAttr(resourceName, "replication", "2"),
					resource.TestCheckResourceAttr(resourceName, "termination_protection", "false"),
					resource.TestCheckResourceAttr(resourceName, "retention_hours", "100"),
				),
			},
			// termination protection test
			{
				Config:                    testAccKafkaTopicTerminationProtectionResource(rName4),
				PreventPostDestroyRefresh: true,
				ExpectNonEmptyPlan:        true,
				PlanOnly:                  true,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "project", os.Getenv("AIVEN_PROJECT_NAME")),
					resource.TestCheckResourceAttr(resourceName, "service_name", fmt.Sprintf("test-acc-sr-%s", rName4)),
					resource.TestCheckResourceAttr(resourceName, "topic_name", fmt.Sprintf("test-acc-topic-%s", rName4)),
					resource.TestCheckResourceAttr(resourceName, "partitions", "3"),
					resource.TestCheckResourceAttr(resourceName, "replication", "2"),
					resource.TestCheckResourceAttr(resourceName, "termination_protection", "true"),
					resource.TestCheckNoResourceAttr(resourceName, "retention_hours"),
				),
			},
		},
	})
}

func TestAccAivenKafkaTopic_450topics(t *testing.T) {
	if os.Getenv("AIVEN_ACC_LONG") == "" {
		t.Skip("Acceptance tests skipped unless env AIVEN_ACC_LONG set")
	}

	resourceName := "aiven_kafka_topic.foo"
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckAivenKafkaTopicResourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccKafka451TopicResource(rName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAivenKafkaTopicAttributes("data.aiven_kafka_topic.topic"),
					resource.TestCheckResourceAttr(resourceName, "project", os.Getenv("AIVEN_PROJECT_NAME")),
					resource.TestCheckResourceAttr(resourceName, "service_name", fmt.Sprintf("test-acc-sr-%s", rName)),
					resource.TestCheckResourceAttr(resourceName, "topic_name", fmt.Sprintf("test-acc-topic-%s", rName)),
					resource.TestCheckResourceAttr(resourceName, "partitions", "3"),
					resource.TestCheckResourceAttr(resourceName, "replication", "2"),
				),
			},
		},
	})
}

func testAccKafka451TopicResource(name string) string {
	s := testAccKafkaTopicResource(name)

	// add extra 100 Kafka topics to test caching layer and creation waiter functionality
	for i := 1; i < 450; i++ {
		s += fmt.Sprintf(`
			resource "aiven_kafka_topic" "foo%d" {
				project = data.aiven_project.foo.project
				service_name = aiven_service.bar.service_name
				topic_name = "test-acc-topic-%d"
				partitions = 3
				replication = 2
			}
		`,
			i, i)
	}

	return s
}

func testAccKafkaTopicResource(name string) string {
	return fmt.Sprintf(`
		data "aiven_project" "foo" {
			project = "%s"
		}

		resource "aiven_service" "bar" {
			project = data.aiven_project.foo.project
			cloud_name = "google-europe-west1"
			plan = "business-4"
			service_name = "test-acc-sr-%s"
			service_type = "kafka"
			maintenance_window_dow = "monday"
			maintenance_window_time = "10:00:00"
		}
		
		resource "aiven_kafka_topic" "foo" {
			project = data.aiven_project.foo.project
			service_name = aiven_service.bar.service_name
			topic_name = "test-acc-topic-%s"
			partitions = 3
			replication = 2
			retention_hours = 1
			retention_bytes = -1
			minimum_in_sync_replicas = 2

			config {
				flush_ms = 10
				unclean_leader_election_enable = true
				cleanup_policy = "compact"
				min_cleanable_dirty_ratio = 0.01
				delete_retention_ms = 50000
			}
		}

		data "aiven_kafka_topic" "topic" {
			project = aiven_kafka_topic.foo.project
			service_name = aiven_kafka_topic.foo.service_name
			topic_name = aiven_kafka_topic.foo.topic_name

			depends_on = [aiven_kafka_topic.foo]
		}
		`, os.Getenv("AIVEN_PROJECT_NAME"), name, name)
}

func testAccKafkaTopicCustomTimeoutsResource(name string) string {
	return fmt.Sprintf(`
		data "aiven_project" "foo" {
			project = "%s"
		}

		resource "aiven_service" "bar" {
			project = data.aiven_project.foo.project
			cloud_name = "google-europe-west1"
			plan = "business-4"
			service_name = "test-acc-sr-%s"
			service_type = "kafka"
			maintenance_window_dow = "monday"
			maintenance_window_time = "10:00:00"

			timeouts {
				create = "25m"
				update = "20m"
			}
		}
		
		resource "aiven_kafka_topic" "foo" {
			project = data.aiven_project.foo.project
			service_name = aiven_service.bar.service_name
			topic_name = "test-acc-topic-%s"
			partitions = 3
			replication = 2
			retention_hours = 100

			timeouts {
				create = "15m"
				read = "15m"
			}
		}

		data "aiven_kafka_topic" "topic" {
			project = aiven_kafka_topic.foo.project
			service_name = aiven_kafka_topic.foo.service_name
			topic_name = aiven_kafka_topic.foo.topic_name

			depends_on = [aiven_kafka_topic.foo]
		}
		`, os.Getenv("AIVEN_PROJECT_NAME"), name, name)
}

func testAccKafkaTopicTerminationProtectionResource(name string) string {
	return fmt.Sprintf(`
		data "aiven_project" "foo" {
			project = "%s"
		}

		resource "aiven_service" "bar" {
			project = data.aiven_project.foo.project
			cloud_name = "google-europe-west1"
			plan = "business-4"
			service_name = "test-acc-sr-%s"
			service_type = "kafka"
			maintenance_window_dow = "monday"
			maintenance_window_time = "10:00:00"
			
			kafka_user_config {
				kafka {
				  group_max_session_timeout_ms = 70000
				  log_retention_bytes = 1000000000
				}
			}
		}
		
		resource "aiven_kafka_topic" "foo" {
			project = data.aiven_project.foo.project
			service_name = aiven_service.bar.service_name
			topic_name = "test-acc-topic-%s"
			partitions = 3
			replication = 2
			termination_protection = true
		}

		data "aiven_kafka_topic" "topic" {
			project = aiven_kafka_topic.foo.project
			service_name = aiven_kafka_topic.foo.service_name
			topic_name = aiven_kafka_topic.foo.topic_name

			depends_on = [aiven_kafka_topic.foo]
		}
		`, os.Getenv("AIVEN_PROJECT_NAME"), name, name)
}

func testAccCheckAivenKafkaTopicAttributes(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		r := s.RootModule().Resources[n]
		a := r.Primary.Attributes

		log.Printf("[DEBUG] kafka topic attributes %v", a)

		if a["project"] == "" {
			return fmt.Errorf("expected to get a project name from Aiven")
		}

		if a["service_name"] == "" {
			return fmt.Errorf("expected to get a service_name from Aiven")
		}

		if a["topic_name"] == "" {
			return fmt.Errorf("expected to get a topic_name from Aiven")
		}

		if a["partitions"] == "" {
			return fmt.Errorf("expected to get partitions from Aiven")
		}

		if a["replication"] == "" {
			return fmt.Errorf("expected to get a replication from Aiven")
		}

		return nil
	}
}

func testAccCheckAivenKafkaTopicResourceDestroy(s *terraform.State) error {
	c := testAccProvider.Meta().(*aiven.Client)

	// loop through the resources in state, verifying each kafka topic is destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "aiven_kafka_topic" {
			continue
		}

		project, serviceName, topicName := splitResourceID3(rs.Primary.ID)

		_, err := c.Services.Get(project, serviceName)
		if err != nil {
			if aiven.IsNotFound(err) {
				return nil
			}
			return err
		}

		t, err := c.KafkaTopics.Get(project, serviceName, topicName)
		if err != nil {
			if aiven.IsNotFound(err) {
				return nil
			}
			return err
		}

		if t != nil {
			return fmt.Errorf("kafka topic (%s) still exists, id %s", topicName, rs.Primary.ID)
		}
	}

	return nil
}

func Test_partitions(t *testing.T) {
	type args struct {
		numPartitions int
	}
	tests := []struct {
		name           string
		args           args
		wantPartitions []*aiven.Partition
	}{
		{
			"basic",
			args{numPartitions: 3},
			[]*aiven.Partition{
				{},
				{},
				{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotPartitions := partitions(tt.args.numPartitions); !reflect.DeepEqual(gotPartitions, tt.wantPartitions) {
				t.Errorf("partitions() = %v, want %v", gotPartitions, tt.wantPartitions)
			}
		})
	}
}
