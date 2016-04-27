package azurerm

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceArmStorageBlob() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmStorageBlobCreate,
		Read:   resourceArmStorageBlobRead,
		Exists: resourceArmStorageBlobExists,
		Delete: resourceArmStorageBlobDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"resource_group_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"storage_account_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"storage_container_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"type": &schema.Schema{
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateArmStorageBlobType,
			},
			"content": &schema.Schema{
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"size", "source"},
			},
			"source": &schema.Schema{
				Type:          schema.TypeString,
				Optional:      true,
				ForceNew:      true,
				ConflictsWith: []string{"size", "content"},
			},
			"size": &schema.Schema{
				Type:          schema.TypeInt,
				Optional:      true,
				ForceNew:      true,
				Default:       0,
				ValidateFunc:  validateArmStorageBlobSize,
				ConflictsWith: []string{"content", "source"},
			},
			"url": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func validateArmStorageBlobSize(v interface{}, k string) (ws []string, errors []error) {
	value := v.(int)

	if value%512 != 0 {
		errors = append(errors, fmt.Errorf("Blob Size %q is invalid, must be a multiple of 512", value))
	}

	return
}

func validateArmStorageBlobType(v interface{}, k string) (ws []string, errors []error) {
	value := strings.ToLower(v.(string))
	validTypes := map[string]struct{}{
		"block": struct{}{},
		"page":  struct{}{},
	}

	if _, ok := validTypes[value]; !ok {
		errors = append(errors, fmt.Errorf("Blob type %q is invalid, must be %q or %q", value, "block", "page"))
	}
	return
}

func resourceArmStorageBlobCreate(d *schema.ResourceData, meta interface{}) error {
	armClient := meta.(*ArmClient)

	resourceGroupName := d.Get("resource_group_name").(string)
	storageAccountName := d.Get("storage_account_name").(string)

	blobClient, err := armClient.getBlobStorageClientForStorageAccount(resourceGroupName, storageAccountName)
	if err != nil {
		return err
	}

	name := d.Get("name").(string)
	blobType := d.Get("type").(string)
	cont := d.Get("storage_container_name").(string)
	var media io.Reader = nil

	if v, ok := d.GetOk("source"); ok {
		err := error(nil)
		media, err = os.Open(v.(string))
		if err != nil {
			return err
		}
	} else if v, ok := d.GetOk("content"); ok {
		media = bytes.NewReader([]byte(v.(string)))
	}

	log.Printf("[INFO] Creating blob %q in storage account %q", name, storageAccountName)
	switch strings.ToLower(blobType) {
	case "block":
		err = blobClient.CreateBlockBlob(cont, name)
		if err != nil {
			return fmt.Errorf("Error creating storage blob on Azure: %s", err)
		}
		if media != nil {
			blockSize := 4 << 20
			blockList := []storage.Block{}
			buffer := make([]byte, blockSize)
			blockNumber := 0
			for {
				n, err := media.Read(buffer)
				if err == io.EOF {
					break
				} else if err != nil {
					return fmt.Errorf("Error creating storage blob on Azure: %s", err)
				}

				blockNumber++
				blockID := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("blobid-%d", blockNumber)))
				err = blobClient.PutBlock(cont, name, blockID, buffer[:n])
				if err != nil {
					return fmt.Errorf("Error creating storage blob on Azure: %s", err)
				}

				blockList = append(blockList,
					storage.Block{
						ID:     blockID,
						Status: storage.BlockStatusLatest,
					},
				)
			}

			err = blobClient.PutBlockList(cont, name, blockList)
			if err != nil {
				return fmt.Errorf("Error creating storage blob on Azure: %s", err)
			}
		}
	case "page":
		size := int64(d.Get("size").(int))
		err = blobClient.PutPageBlob(cont, name, size, map[string]string{})
		if err != nil {
			return fmt.Errorf("Error creating storage blob on Azure: %s", err)
		}
		if media != nil {
			// do the upload
		}
	}

	d.SetId(name)
	return resourceArmStorageBlobRead(d, meta)
}

func resourceArmStorageBlobRead(d *schema.ResourceData, meta interface{}) error {
	armClient := meta.(*ArmClient)

	resourceGroupName := d.Get("resource_group_name").(string)
	storageAccountName := d.Get("storage_account_name").(string)

	blobClient, err := armClient.getBlobStorageClientForStorageAccount(resourceGroupName, storageAccountName)
	if err != nil {
		return err
	}

	exists, err := resourceArmStorageBlobExists(d, meta)
	if err != nil {
		return err
	}

	if !exists {
		// Exists already removed this from state
		return nil
	}

	name := d.Get("name").(string)
	storageContainerName := d.Get("storage_container_name").(string)

	url := blobClient.GetBlobURL(storageContainerName, name)
	if url == "" {
		log.Printf("[INFO] URL for %q is empty", name)
	}
	d.Set("url", url)

	return nil
}

func resourceArmStorageBlobExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	armClient := meta.(*ArmClient)

	resourceGroupName := d.Get("resource_group_name").(string)
	storageAccountName := d.Get("storage_account_name").(string)

	blobClient, err := armClient.getBlobStorageClientForStorageAccount(resourceGroupName, storageAccountName)
	if err != nil {
		return false, err
	}

	name := d.Get("name").(string)
	storageContainerName := d.Get("storage_container_name").(string)

	log.Printf("[INFO] Checking for existence of storage blob %q.", name)
	exists, err := blobClient.BlobExists(storageContainerName, name)
	if err != nil {
		return false, fmt.Errorf("error testing existence of storage blob %q: %s", name, err)
	}

	if !exists {
		log.Printf("[INFO] Storage blob %q no longer exists, removing from state...", name)
		d.SetId("")
	}

	return exists, nil
}

func resourceArmStorageBlobDelete(d *schema.ResourceData, meta interface{}) error {
	armClient := meta.(*ArmClient)

	resourceGroupName := d.Get("resource_group_name").(string)
	storageAccountName := d.Get("storage_account_name").(string)

	blobClient, err := armClient.getBlobStorageClientForStorageAccount(resourceGroupName, storageAccountName)
	if err != nil {
		return err
	}

	name := d.Get("name").(string)
	storageContainerName := d.Get("storage_container_name").(string)

	log.Printf("[INFO] Deleting storage blob %q", name)
	if _, err = blobClient.DeleteBlobIfExists(storageContainerName, name); err != nil {
		return fmt.Errorf("Error deleting storage blob %q: %s", name, err)
	}

	d.SetId("")
	return nil
}
