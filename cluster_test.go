package cluster

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("PartitionSlice", func() {

	It("should sort correctly", func() {
		p1 := Partition{Addr: "host1:9093", ID: 1}
		p2 := Partition{Addr: "host1:9092", ID: 2}
		p3 := Partition{Addr: "host2:9092", ID: 3}
		p4 := Partition{Addr: "host3:9091", ID: 4}
		p5 := Partition{Addr: "host2:9093", ID: 5}
		p6 := Partition{Addr: "host1:9092", ID: 6}

		slice := PartitionSlice{p1, p2, p3, p4, p5, p6}
		sort.Sort(slice)
		Expect(slice).To(BeEquivalentTo(PartitionSlice{p2, p6, p1, p3, p5, p4}))
	})

})

/*********************************************************************
 * TEST HOOK
 *********************************************************************/

const (
	t_KAFKA_VERSION = "kafka_2.10-0.8.1.1"
	t_CLIENT        = "sarama-cluster-client"
	t_TOPIC         = "sarama-cluster-topic"
	t_GROUP         = "sarama-cluster-group"
	t_DIR           = "/tmp/sarama-cluster-test"
)

var _ = BeforeSuite(func() {
	runner := testDir(t_KAFKA_VERSION, "bin", "kafka-run-class.sh")
	testState.zookeeper = exec.Command(runner, "-name", "zookeeper", "org.apache.zookeeper.server.ZooKeeperServerMain", testDir("zookeeper.properties"))
	testState.kafka = exec.Command(runner, "-name", "kafkaServer", "kafka.Kafka", testDir("server.properties"))
	testState.kafka.Env = []string{"KAFKA_HEAP_OPTS=-Xmx1G -Xms1G"}

	// Create Dir
	Expect(os.MkdirAll(t_DIR, 0775)).NotTo(HaveOccurred())

	// Start ZK
	Expect(testState.zookeeper.Start()).NotTo(HaveOccurred())
	Eventually(func() *os.Process {
		return testState.zookeeper.Process
	}).ShouldNot(BeNil())

	// Start Kafka
	Expect(testState.kafka.Start()).NotTo(HaveOccurred())
	Eventually(func() *os.Process {
		return testState.kafka.Process
	}).ShouldNot(BeNil())
	time.Sleep(3 * time.Second)

	// Create and wait for client
	client, err := newClient()
	Expect(err).NotTo(HaveOccurred())
	defer client.Close()

	Eventually(func() error {
		_, err := client.Partitions(t_TOPIC)
		return err
	}).ShouldNot(HaveOccurred(), "10s")

	// Seed messages
	Expect(seedMessages(client, 10000)).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if testState.kafka != nil {
		testState.kafka.Process.Kill()
	}
	if testState.zookeeper != nil {
		testState.zookeeper.Process.Kill()
	}
	Expect(os.RemoveAll(t_DIR)).NotTo(HaveOccurred())
})

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeEach(func() {
		testState.notifier = &mockNotifier{messages: make([]string, 0)}
	})
	RunSpecs(t, "sarama/cluster")
}

/*******************************************************************
 * TEST HELPERS
 *******************************************************************/

var testState struct {
	kafka, zookeeper *exec.Cmd
	notifier         *mockNotifier
}

func newClient() (*sarama.Client, error) {
	return sarama.NewClient(t_CLIENT, []string{"127.0.0.1:29092"}, sarama.NewClientConfig())
}

func testDir(tokens ...string) string {
	_, filename, _, _ := runtime.Caller(1)
	tokens = append([]string{path.Dir(filename), "test"}, tokens...)
	return path.Join(tokens...)
}

func seedMessages(client *sarama.Client, count int) error {
	producer, err := sarama.NewSimpleProducer(client, t_TOPIC, nil)
	if err != nil {
		return err
	}
	defer producer.Close()

	for i := 0; i < count; i++ {
		kv := sarama.StringEncoder(fmt.Sprintf("PLAINDATA-%08d", i))
		err := producer.SendMessage(kv, kv)
		if err != nil {
			return err
		}
	}
	return nil
}

type mockNotifier struct{ messages []string }

func (n *mockNotifier) RebalanceStart(c *Consumer) {
	n.messages = append(n.messages, fmt.Sprintf("rebalance start %s", c.Group()))
}
func (n *mockNotifier) RebalanceOK(c *Consumer) {
	n.messages = append(n.messages, fmt.Sprintf("rebalance ok %s", c.Group()))
}
func (n *mockNotifier) RebalanceError(c *Consumer, err error) {
	n.messages = append(n.messages, fmt.Sprintf("rebalance error %s: %s", c.Group(), err.Error()))
}
