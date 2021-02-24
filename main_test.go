package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2020-08-01/network"
)

func TestReportChangedTags(t *testing.T) {

	aBytes, err := ioutil.ReadFile("testdata/tagsA.json")
	if err != nil {
		t.Fatal(err)
	}
	var tagsA network.ServiceTagsListResult
	json.Unmarshal(aBytes, &tagsA)

	bBytes, err := ioutil.ReadFile("testdata/tagsB.json")
	if err != nil {
		t.Fatal(err)
	}
	var tagsB network.ServiceTagsListResult
	json.Unmarshal(bBytes, &tagsB)

	report, err := reportChangedTags(tagsA, tagsB)
	if err != nil {
		t.Fatal(fmt.Printf("[ERROR] reporting changed tags: %s", err))
	}
	fmt.Println(report)
}
