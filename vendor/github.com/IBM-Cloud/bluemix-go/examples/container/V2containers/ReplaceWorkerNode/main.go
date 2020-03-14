package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	bluemix "github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/IBM-Cloud/bluemix-go/trace"

	v2 "github.com/IBM-Cloud/bluemix-go/api/container/containerv2"
)

func main() {

	var clusterID string
	flag.StringVar(&clusterID, "clusterID", "", "Cluster ID or Name")

	var workerID string
	flag.StringVar(&workerID, "workerID", "", "worker ID")

	flag.Parse()

	if clusterID == "" || workerID == "" {
		flag.Usage()
		os.Exit(1)
	}
	c := new(bluemix.Config)

	trace.Logger = trace.NewLogger("true")

	sess, err := session.New(c)
	if err != nil {
		log.Fatal(err)
	}

	if err != nil {
		log.Fatal(err)
	}

	target := v2.ClusterTargetHeader{}

	clusterClient, err := v2.New(sess)
	if err != nil {
		log.Fatal(err)
	}
	workersAPI := clusterClient.Workers()

	out, err := workersAPI.ReplaceWokerNode(clusterID, workerID, target)

	fmt.Println("out=", out)
}
