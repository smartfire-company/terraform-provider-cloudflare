package cloudflare

import (
	"fmt"
	"log"
	"strings"

	cloudflare "github.com/cloudflare/cloudflare-go"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
	"github.com/pkg/errors"
)

func resourceCloudflareAccessApplication() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudflareAccessApplicationCreate,
		Read:   resourceCloudflareAccessApplicationRead,
		Update: resourceCloudflareAccessApplicationUpdate,
		Delete: resourceCloudflareAccessApplicationDelete,
		Importer: &schema.ResourceImporter{
			State: resourceCloudflareAccessApplicationImport,
		},

		Schema: map[string]*schema.Schema{
			"account_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"zone_id": {
				Deprecated: "This field will be removed in version 3 and replaced with the account_id field.",
				Type:       schema.TypeString,
				Optional:   true,
				Computed:   true,
			},
			"aud": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"domain": {
				Type:     schema.TypeString,
				Required: true,
			},
			"session_duration": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "24h",
				ValidateFunc: validation.StringInSlice([]string{"0s", "15m", "30m", "6h", "12h", "24h", "168h", "730h"}, false),
			},
			"cors_headers": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"allowed_methods": {
							Type:     schema.TypeSet,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
						"allowed_origins": {
							Type:     schema.TypeSet,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
						"allowed_headers": {
							Type:     schema.TypeSet,
							Optional: true,
							Elem: &schema.Schema{
								Type: schema.TypeString,
							},
						},
						"allow_all_methods": {
							Type:     schema.TypeBool,
							Optional: true,
						},
						"allow_all_origins": {
							Type:     schema.TypeBool,
							Optional: true,
						},
						"allow_all_headers": {
							Type:     schema.TypeBool,
							Optional: true,
						},
						"allow_credentials": {
							Type:     schema.TypeBool,
							Optional: true,
						},
						"max_age": {
							Type:         schema.TypeInt,
							Optional:     true,
							ValidateFunc: validation.IntBetween(-1, 86400),
						},
					},
				},
			},
			"auto_redirect_to_identity": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"allowed_idps": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func resourceCloudflareAccessApplicationCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*cloudflare.API)
	accountID, err := getAccountIDFromZoneID(d, client)
	if err != nil {
		return err
	}

	allowedIDPList := expandInterfaceToStringList(d.Get("allowed_idps"))

	newAccessApplication := cloudflare.AccessApplication{
		Name:                   d.Get("name").(string),
		Domain:                 d.Get("domain").(string),
		SessionDuration:        d.Get("session_duration").(string),
		AutoRedirectToIdentity: d.Get("auto_redirect_to_identity").(bool),
	}

	if len(allowedIDPList) > 0 {
		newAccessApplication.AllowedIdps = allowedIDPList
	}

	if _, ok := d.GetOk("cors_headers"); ok {
		CORSConfig, err := convertCORSSchemaToStruct(d)
		if err != nil {
			return err
		}
		newAccessApplication.CorsHeaders = CORSConfig
	}

	log.Printf("[DEBUG] Creating Cloudflare Access Application from struct: %+v", newAccessApplication)

	accessApplication, err := client.CreateAccessApplication(accountID, newAccessApplication)
	if err != nil {
		return fmt.Errorf("error creating Access Application for account %q: %s", accountID, err)
	}

	d.SetId(accessApplication.ID)
	d.Set("account_id", accountID)

	return resourceCloudflareAccessApplicationRead(d, meta)
}

func resourceCloudflareAccessApplicationRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*cloudflare.API)
	accountID, err := getAccountIDFromZoneID(d, client)
	if err != nil {
		return err
	}

	accessApplication, err := client.AccessApplication(accountID, d.Id())
	if err != nil {
		if strings.Contains(err.Error(), "HTTP status 404") {
			log.Printf("[INFO] Access Application %s no longer exists", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error finding Access Application %q: %s", d.Id(), err)
	}

	d.Set("aud", accessApplication.AUD)
	d.Set("session_duration", accessApplication.SessionDuration)
	d.Set("domain", accessApplication.Domain)
	d.Set("auto_redirect_to_identity", accessApplication.AutoRedirectToIdentity)
	d.Set("allowed_idps", accessApplication.AllowedIdps)

	corsConfig := convertCORSStructToSchema(d, accessApplication.CorsHeaders)
	if corsConfigErr := d.Set("cors_headers", corsConfig); corsConfigErr != nil {
		return fmt.Errorf("error setting Access Application CORS header configuration: %s", corsConfigErr)
	}

	return nil
}

func resourceCloudflareAccessApplicationUpdate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*cloudflare.API)
	accountID, err := getAccountIDFromZoneID(d, client)
	if err != nil {
		return err
	}
	allowedIDPList := expandInterfaceToStringList(d.Get("allowed_idps"))

	updatedAccessApplication := cloudflare.AccessApplication{
		ID:                     d.Id(),
		Name:                   d.Get("name").(string),
		Domain:                 d.Get("domain").(string),
		SessionDuration:        d.Get("session_duration").(string),
		AutoRedirectToIdentity: d.Get("auto_redirect_to_identity").(bool),
	}

	if len(allowedIDPList) > 0 {
		updatedAccessApplication.AllowedIdps = allowedIDPList
	}

	if _, ok := d.GetOk("cors_headers"); ok {
		CORSConfig, err := convertCORSSchemaToStruct(d)
		if err != nil {
			return err
		}
		updatedAccessApplication.CorsHeaders = CORSConfig
	}

	log.Printf("[DEBUG] Updating Cloudflare Access Application from struct: %+v", updatedAccessApplication)

	accessApplication, err := client.UpdateAccessApplication(accountID, updatedAccessApplication)
	if err != nil {
		return fmt.Errorf("error updating Access Application for account %q: %s", accountID, err)
	}

	if accessApplication.ID == "" {
		return fmt.Errorf("failed to find Access Application ID in update response; resource was empty")
	}

	return resourceCloudflareAccessApplicationRead(d, meta)
}

func resourceCloudflareAccessApplicationDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*cloudflare.API)
	accountID, err := getAccountIDFromZoneID(d, client)
	if err != nil {
		return err
	}
	appID := d.Id()

	log.Printf("[DEBUG] Deleting Cloudflare Access Application using ID: %s", appID)

	err = client.DeleteAccessApplication(accountID, appID)
	if err != nil {
		return fmt.Errorf("error deleting Access Application for account %q: %s", accountID, err)
	}

	resourceCloudflareAccessApplicationRead(d, meta)

	return nil
}

func resourceCloudflareAccessApplicationImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	attributes := strings.SplitN(d.Id(), "/", 2)

	if len(attributes) != 2 {
		return nil, fmt.Errorf("invalid id (\"%s\") specified, should be in format \"accountID/accessApplicationID\"", d.Id())
	}

	accountID, accessApplicationID := attributes[0], attributes[1]

	log.Printf("[DEBUG] Importing Cloudflare Access Application: id %s for account %s", accessApplicationID, accountID)

	d.Set("account_id", accountID)
	d.SetId(accessApplicationID)

	resourceCloudflareAccessApplicationRead(d, meta)

	return []*schema.ResourceData{d}, nil
}

func convertCORSSchemaToStruct(d *schema.ResourceData) (*cloudflare.AccessApplicationCorsHeaders, error) {
	CORSConfig := cloudflare.AccessApplicationCorsHeaders{}

	if _, ok := d.GetOk("cors_headers"); ok {
		if allowedMethods, ok := d.GetOk("cors_headers.0.allowed_methods"); ok {
			CORSConfig.AllowedMethods = expandInterfaceToStringList(allowedMethods.(*schema.Set).List())

		}

		if allowedHeaders, ok := d.GetOk("cors_headers.0.allowed_headers"); ok {
			CORSConfig.AllowedHeaders = expandInterfaceToStringList(allowedHeaders.(*schema.Set).List())
		}

		if allowedOrigins, ok := d.GetOk("cors_headers.0.allowed_origins"); ok {
			CORSConfig.AllowedOrigins = expandInterfaceToStringList(allowedOrigins.(*schema.Set).List())
		}

		CORSConfig.AllowAllMethods = d.Get("cors_headers.0.allow_all_methods").(bool)
		CORSConfig.AllowAllHeaders = d.Get("cors_headers.0.allow_all_headers").(bool)
		CORSConfig.AllowAllOrigins = d.Get("cors_headers.0.allow_all_origins").(bool)
		CORSConfig.AllowCredentials = d.Get("cors_headers.0.allow_credentials").(bool)
		CORSConfig.MaxAge = d.Get("cors_headers.0.max_age").(int)

		// Ensure that should someone forget to set allowed methods (either
		// individually or *), we throw an error to prevent getting into an
		// unrecoverable state.
		if CORSConfig.AllowAllOrigins || len(CORSConfig.AllowedOrigins) > 1 {
			if CORSConfig.AllowAllMethods == false && len(CORSConfig.AllowedMethods) == 0 {
				return nil, errors.New("must set allowed_methods or allow_all_methods")
			}
		}

		// Ensure that should someone forget to set allowed origins (either
		// individually or *), we throw an error to prevent getting into an
		// unrecoverable state.
		if CORSConfig.AllowAllMethods || len(CORSConfig.AllowedMethods) > 1 {
			if CORSConfig.AllowAllOrigins == false && len(CORSConfig.AllowedOrigins) == 0 {
				return nil, errors.New("must set allowed_origins or allow_all_origins")
			}
		}
	}

	return &CORSConfig, nil
}

func convertCORSStructToSchema(d *schema.ResourceData, headers *cloudflare.AccessApplicationCorsHeaders) []interface{} {
	if _, ok := d.GetOk("cors_headers"); !ok {
		return []interface{}{}
	}

	m := map[string]interface{}{
		"allow_all_methods": headers.AllowAllMethods,
		"allow_all_headers": headers.AllowAllHeaders,
		"allow_all_origins": headers.AllowAllOrigins,
		"allow_credentials": headers.AllowCredentials,
		"max_age":           headers.MaxAge,
	}

	m["allowed_methods"] = flattenStringList(headers.AllowedMethods)
	m["allowed_headers"] = flattenStringList(headers.AllowedHeaders)
	m["allowed_origins"] = flattenStringList(headers.AllowedOrigins)

	return []interface{}{m}
}
