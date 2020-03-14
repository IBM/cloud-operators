package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/IBM-Cloud/bluemix-go/api/iamuum/iamuumv2"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/IBM-Cloud/bluemix-go/trace"
)

func main() {
	var agID string
	flag.StringVar(&agID, "agID", "", "Access group ID")

	flag.Parse()
	if agID == "" {
		flag.Usage()
		os.Exit(1)
	}

	trace.Logger = trace.NewLogger("true")
	sess, err := session.New()
	if err != nil {
		log.Fatal(err)
	}

	iamuumClient, err := iamuumv2.New(sess)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(agID)

	drAPI := iamuumClient.DynamicRule()

	rulereq := iamuumv2.CreateRuleRequest{
		Name:       "test rule name 12",
		Expiration: 24,
		RealmName:  "test-idp.com",
		Conditions: []iamuumv2.Condition{
			{
				Claim:    "blueGroups",
				Operator: "CONTAINS",
				Value:    "\"test-bluegroup-saml\"",
			},
		},
	}

	resp, err := drAPI.Create(agID, rulereq)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("create resp=", resp)

	listres, err := drAPI.List(agID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nlistres=", listres)

	getres, etag, err := drAPI.Get(agID, listres[0].RuleID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\ngetres=", getres)

	upres, err := drAPI.Replace(agID, listres[0].RuleID, rulereq, etag)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nupres=", upres)

	err = drAPI.Delete(agID, listres[0].RuleID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nerr=", err)
}
