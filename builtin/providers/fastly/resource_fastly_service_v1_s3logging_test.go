package fastly

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform/helper/acctest"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/terraform"
	gofastly "github.com/sethvargo/go-fastly"
)

func TestAccFastlyServiceV1_s3logging_basic(t *testing.T) {
	var service gofastly.ServiceDetail
	name := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domainName1 := fmt.Sprintf("%s.notadomain.com", acctest.RandString(10))

	log1 := gofastly.S3{
		Version:         "1",
		Name:            "somebucketlog",
		BucketName:      "fastlytestlogging",
		Domain:          "s3-us-west-2.amazonaws.com",
		AccessKey:       "somekey",
		SecretKey:       "somesecret",
		Period:          uint(3600),
		GzipLevel:       uint(0),
		Format:          "%h %l %u %t %r %>s",
		TimestampFormat: "%Y-%m-%dT%H:%M:%S.000",
	}

	log2 := gofastly.S3{
		Version:         "1",
		Name:            "someotherbucketlog",
		BucketName:      "fastlytestlogging2",
		Domain:          "s3-us-west-2.amazonaws.com",
		AccessKey:       "someotherkey",
		SecretKey:       "someothersecret",
		GzipLevel:       uint(3),
		Period:          uint(60),
		Format:          "%h %l %u %t %r %>s",
		TimestampFormat: "%Y-%m-%dT%H:%M:%S.000",
	}

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckServiceV1Destroy,
		Steps: []resource.TestStep{
			resource.TestStep{
				Config: testAccServiceV1S3LoggingConfig(name, domainName1),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServiceV1Exists("fastly_service_v1.foo", &service),
					testAccCheckFastlyServiceV1S3LoggingAttributes(&service, []*gofastly.S3{&log1}),
					resource.TestCheckResourceAttr(
						"fastly_service_v1.foo", "name", name),
					resource.TestCheckResourceAttr(
						"fastly_service_v1.foo", "s3logging.#", "1"),
				),
			},

			resource.TestStep{
				Config: testAccServiceV1S3LoggingConfig_update(name, domainName1),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServiceV1Exists("fastly_service_v1.foo", &service),
					testAccCheckFastlyServiceV1S3LoggingAttributes(&service, []*gofastly.S3{&log1, &log2}),
					resource.TestCheckResourceAttr(
						"fastly_service_v1.foo", "name", name),
					resource.TestCheckResourceAttr(
						"fastly_service_v1.foo", "s3logging.#", "2"),
				),
			},
		},
	})
}

func testAccCheckFastlyServiceV1S3LoggingAttributes(service *gofastly.ServiceDetail, s3s []*gofastly.S3) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		conn := testAccProvider.Meta().(*FastlyClient).conn
		s3List, err := conn.ListS3s(&gofastly.ListS3sInput{
			Service: service.ID,
			Version: service.ActiveVersion.Number,
		})

		if err != nil {
			return fmt.Errorf("[ERR] Error looking up S3 Logging for (%s), version (%s): %s", service.Name, service.ActiveVersion.Number, err)
		}

		if len(s3List) != len(s3s) {
			return fmt.Errorf("S3 List count mismatch, expected (%d), got (%d)", len(s3s), len(s3List))
		}

		var found int
		for _, s := range s3s {
			for _, ls := range s3List {
				if s.Name == ls.Name {
					found++
					// we don't know these things ahead of time, so populate them now
					s.ServiceID = service.ID
					s.Version = service.ActiveVersion.Number
					// We don't track these, so clear them out because we also wont know
					// these ahead of time
					ls.CreatedAt = nil
					ls.UpdatedAt = nil
					if !reflect.DeepEqual(s, ls) {
						return fmt.Errorf("Bad match S3 logging match, expected (%#v), got (%#v)", s, ls)
					}
				}
			}
		}

		if found != len(s3s) {
			return fmt.Errorf("Error matching S3 Logging rules")
		}

		return nil
	}
}

func testAccServiceV1S3LoggingConfig(name, domain string) string {
	return fmt.Sprintf(`
resource "fastly_service_v1" "foo" {
  name = "%s"

  domain {
    name    = "%s"
    comment = "tf-testing-domain"
  }

  backend {
    address = "aws.amazon.com"
    name    = "amazon docs"
  }

  s3logging {
    name       = "somebucketlog"
                bucket_name = "fastlytestlogging"
    domain     = "s3-us-west-2.amazonaws.com"
                s3_access_key = "somekey"
    s3_secret_key = "somesecret"
  }

  force_destroy = true
}`, name, domain)
}

func testAccServiceV1S3LoggingConfig_update(name, domain string) string {
	return fmt.Sprintf(`
resource "fastly_service_v1" "foo" {
  name = "%s"

  domain {
    name    = "%s"
    comment = "tf-testing-domain"
  }

  backend {
    address = "aws.amazon.com"
    name    = "amazon docs"
  }

  s3logging {
    name          = "somebucketlog"
    bucket_name   = "fastlytestlogging"
    domain        = "s3-us-west-2.amazonaws.com"
    s3_access_key = "somekey"
    s3_secret_key = "somesecret"
  }

  s3logging {
    name          = "someotherbucketlog"
    bucket_name   = "fastlytestlogging2"
    domain        = "s3-us-west-2.amazonaws.com"
    s3_access_key = "someotherkey"
    s3_secret_key = "someothersecret"
    period        = 60
    gzip_level    = 3
  }

  force_destroy = true
}`, name, domain)
}
