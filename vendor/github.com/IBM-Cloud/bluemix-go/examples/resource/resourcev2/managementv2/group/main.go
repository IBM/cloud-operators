package main

import (
	"flag"
	"log"
	"os"

	bluemix "github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/api/resource/resourcev2/managementv2"
	"github.com/IBM-Cloud/bluemix-go/models"
	"github.com/IBM-Cloud/bluemix-go/session"
	"github.com/IBM-Cloud/bluemix-go/trace"
)

func main() {

	var resourcegrp string
	flag.StringVar(&resourcegrp, "name", "mynewgroup", "Name of the group")

	flag.Parse()

	c := new(bluemix.Config)

	if resourcegrp == "" {
		flag.Usage()
		os.Exit(1)
	}

	// resourcegrp = "mynewgroup"

	trace.Logger = trace.NewLogger("true")
	sess, err := session.New(c)
	if err != nil {
		log.Fatal(err)
	}

	client, err := managementv2.New(sess)
	if err != nil {
		log.Fatal(err)
	}

	resGrpAPI := client.ResourceGroup()

	resourceGroupQuery := managementv2.ResourceGroupQuery{
		Default: true,
	}

	grpList, err := resGrpAPI.List(&resourceGroupQuery)

	if err != nil {
		log.Fatal(err)
	}

	log.Println("\nResource group default Details :", grpList)

	// var name = models.ResourceGroup{
	// 	Name: resourcegrp,
	// }
	var grpInfo = models.ResourceGroupv2{
		ResourceGroup: models.ResourceGroup{
			Name: resourcegrp,
		},
	}

	grp, err := resGrpAPI.Create(grpInfo)

	if err != nil {
		log.Fatal(err)
	}

	log.Println("\nResource group create :", grp)

	grps, err := resGrpAPI.FindByName(nil, resourcegrp)

	if err != nil {
		log.Fatal(err)
	}

	log.Println("\nResource group Details :", grps[0])

	grp, err = resGrpAPI.Get(grp.ID)

	if err != nil {
		log.Fatal(err)
	}

	log.Println("\nResource group Details by ID:", grp)

	var updateGrpInfo = managementv2.ResourceGroupUpdateRequest{
		Name: "default",
	}

	grp, err = resGrpAPI.Update(grp.ID, &updateGrpInfo)

	if err != nil {
		log.Fatal(err)
	}

	log.Println("\nResource group update :", grp)

	err = resGrpAPI.Delete(grp.ID)
	if err != nil {
		log.Fatal(err)
	}

}
