package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2020-08-01/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/go-azure-helpers/authentication"
	"github.com/tombuildsstuff/giovanni/storage/2019-12-12/blob/blobs"
)

// Client is struct for Azure resources
type Client struct {
	BlobsClient       blobs.Client
	ServiceTagsClient network.ServiceTagsClient
}

func buildClient() (*Client, error) {
	// use Giovanni style github.com/tombuildsstuff/giovanni
	//
	// we're using github.com/hashicorp/go-azure-helpers since it makes this simpler
	// but you can use an Authorizer from github.com/Azure/go-autorest directly too
	builder := &authentication.Builder{
		SubscriptionID: os.Getenv("AZURE_SUBSCRIPTION_ID"),
		ClientID:       os.Getenv("AZURE_CLIENT_ID"),
		ClientSecret:   os.Getenv("AZURE_CLIENT_SECRET"),
		TenantID:       os.Getenv("AZURE_TENANT_ID"),
		Environment:    os.Getenv("AZURE_ENVIRONMENT"),

		// Feature Toggles
		SupportsClientSecretAuth:       true,
		SupportsManagedServiceIdentity: true,
	}

	config, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("Error building AzureRM Client: %s", err)
	}

	env, err := authentication.DetermineEnvironment(config.Environment)
	if err != nil {
		return nil, err
	}

	oauthConfig, err := config.BuildOAuthConfig(env.ActiveDirectoryEndpoint)
	if err != nil {
		return nil, err
	}

	// OAuthConfigForTenant returns a pointer, which can be nil.
	if oauthConfig == nil {
		return nil, fmt.Errorf("Unable to configure OAuthConfig for tenant %s", config.TenantID)
	}

	// support for HTTP Proxies
	sender := autorest.DecorateSender(&http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	})

	auth, err := config.GetAuthorizationToken(sender, oauthConfig, "https://management.azure.com/")

	storageAuth, err := config.GetAuthorizationToken(sender, oauthConfig, "https://storage.azure.com/")
	if err != nil {
		return nil, err
	}

	blobsClient := blobs.New()
	blobsClient.Client.Authorizer = storageAuth

	serviceTagsClient := network.NewServiceTagsClient(os.Getenv("AZURE_SUBSCRIPTION_ID"))
	if err != nil {
		return nil, err
	}
	serviceTagsClient.Authorizer = auth

	result := &Client{
		BlobsClient:       blobsClient,
		ServiceTagsClient: serviceTagsClient,
	}

	return result, nil
}

func reportChangedTags(tagsA network.ServiceTagsListResult, tagsB network.ServiceTagsListResult) (string, error) {
	isDetected := false
	var report string

	opt := cmpopts.SortSlices(func(i, j string) bool {
		return i < j
	})

	for _, a := range *tagsA.Values {
		for _, b := range *tagsB.Values {
			if cmp.Equal(a.ID, b.ID) {
				if diff := cmp.Diff(a.Properties.AddressPrefixes, b.Properties.AddressPrefixes, opt); diff != "" {
					report += fmt.Sprintf("Changed ID: %s\n", *a.ID)
					report += fmt.Sprintf("Change Number: %s -> %s\n", *a.Properties.ChangeNumber, *b.Properties.ChangeNumber)
					report += fmt.Sprintln(strings.Replace(diff, "&[]string", "AddressPrefixes", 1))
					isDetected = true
				}
			}
		}
	}

	if !isDetected {
		return "", fmt.Errorf("Changed tags not found")
	}
	return report, nil
}

func putServiceTagsBlobs(ctx context.Context, client Client, storageAccountName string, containerName string, latestBlobName string, tags network.ServiceTagsListResult) error {
	changeNumber := *tags.ChangeNumber
	st, _ := json.Marshal(tags)
	c := []byte(st)
	ct := "application/json"
	putBlockBlobInput := blobs.PutBlockBlobInput{
		Content:     &c,
		ContentType: &ct,
	}

	archiveBlobName := fmt.Sprintf("changenumber-%s.json", changeNumber)

	log.Printf("Putting blob.. (for archiving)")
	if _, err := client.BlobsClient.PutBlockBlob(ctx, storageAccountName, containerName, archiveBlobName, putBlockBlobInput); err != nil {
		return fmt.Errorf("Error putting blob: %s", err)
	}

	log.Printf("Putting blob.. (for saving/updating the latest)")
	if _, err := client.BlobsClient.PutBlockBlob(ctx, storageAccountName, containerName, latestBlobName, putBlockBlobInput); err != nil {
		return fmt.Errorf("Error putting blob: %s", err)

	}

	return nil
}

func putReportBlobs(ctx context.Context, client Client, storageAccountName string, containerName string, report string, changeNumber string) error {
	c := []byte(report)
	ct := "text/plain"
	putBlockBlobInput := blobs.PutBlockBlobInput{
		Content:     &c,
		ContentType: &ct,
	}

	reportBlobName := fmt.Sprintf("changenumber-%s-report.txt", changeNumber)

	log.Printf("Putting blob.. (for report)")
	if _, err := client.BlobsClient.PutBlockBlob(ctx, storageAccountName, containerName, reportBlobName, putBlockBlobInput); err != nil {
		return fmt.Errorf("Error putting blob: %s", err)
	}

	return nil
}

// Run is a real main
func Run() int {
	log.Printf("Started..")

	storageAccountName := os.Getenv("SERVICETAGS_CHECK_STORAGE_ACCOUNT_NAME")
	if storageAccountName == "" {
		log.Printf("[ERROR] need to set env. var: SERVICETAGS_CHECK_STORAGE_ACCOUNT_NAME")
		return 1
	}
	serviceTagsContainerName := "servicetags"
	reportContainerName := "servicetags-report"
	latestBlobName := "latest.json"
	// not for filtering (will get tags of all regions), but change number consistency
	serviceTagsAPILoc := os.Getenv("SERVICETAGS_CHECK_API_LOC")
	if serviceTagsAPILoc == "" {
		serviceTagsAPILoc = "japaneast"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("Building Client..")
	client, err := buildClient()
	if err != nil {
		log.Printf(fmt.Sprintf("[ERROR] building client: %s", err))
		return 1
	}

	log.Printf("Getting Service Tags from API..")
	tagsFromAPI, err := client.ServiceTagsClient.List(ctx, serviceTagsAPILoc)
	if err != nil {
		log.Printf(fmt.Sprintf("[ERROR] getting Service Tags: %s", err))
		return 1
	}

	log.Printf("Getting my latest Service Tags from blob..")
	resp, err := client.BlobsClient.Get(ctx, storageAccountName, serviceTagsContainerName, latestBlobName, blobs.GetInput{})
	if err != nil {
		if resp.StatusCode == 404 {
			log.Printf("Service tags blob not found. Putting blob for the next check")
			if err := putServiceTagsBlobs(ctx, *client, storageAccountName, serviceTagsContainerName, latestBlobName, tagsFromAPI); err != nil {
				log.Printf(fmt.Sprintf("[ERROR] putting blob: %s", err))
			}
			return 0
		}
		log.Printf(fmt.Sprintf("[ERROR] getting blob: %s", err))
		return 1
	}

	var tagsFromBlob network.ServiceTagsListResult
	json.Unmarshal(resp.Contents, &tagsFromBlob)

	if cmp.Equal(tagsFromBlob.ChangeNumber, tagsFromAPI.ChangeNumber) {
		log.Printf("Service tag is the same as last check")
		return 0
	}
	log.Printf("Service tag has been changed. Change No. %s -> %v\n", *tagsFromBlob.ChangeNumber, *tagsFromAPI.ChangeNumber)

	report, err := reportChangedTags(tagsFromBlob, tagsFromAPI)
	if err != nil {
		log.Printf(fmt.Sprintf("[ERROR] making changed tags report: %s", err))
		return 1
	}

	if err := putServiceTagsBlobs(ctx, *client, storageAccountName, serviceTagsContainerName, latestBlobName, tagsFromAPI); err != nil {
		log.Printf(fmt.Sprintf("[ERROR] putting service tags blobs: %s", err))
		return 1
	}

	if err := putReportBlobs(ctx, *client, storageAccountName, reportContainerName, report, *tagsFromAPI.ChangeNumber); err != nil {
		log.Printf(fmt.Sprintf("[ERROR] putting report blob: %s", err))
		return 1
	}

	return 0

}

func main() { os.Exit(Run()) }
