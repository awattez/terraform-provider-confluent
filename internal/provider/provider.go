// Copyright 2021 Confluent Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"context"
	"fmt"
	ccp "github.com/confluentinc/ccloud-sdk-go-v2/connect-custom-plugin/v1"
	netip "github.com/confluentinc/ccloud-sdk-go-v2/networking-ip/v1"
	"strings"

	apikeys "github.com/confluentinc/ccloud-sdk-go-v2/apikeys/v2"
	byok "github.com/confluentinc/ccloud-sdk-go-v2/byok/v1"
	cmk "github.com/confluentinc/ccloud-sdk-go-v2/cmk/v2"
	connect "github.com/confluentinc/ccloud-sdk-go-v2/connect/v1"
	fcpm "github.com/confluentinc/ccloud-sdk-go-v2/flink/v2"
	iamv1 "github.com/confluentinc/ccloud-sdk-go-v2/iam/v1"
	iam "github.com/confluentinc/ccloud-sdk-go-v2/iam/v2"
	oidc "github.com/confluentinc/ccloud-sdk-go-v2/identity-provider/v2"
	quotas "github.com/confluentinc/ccloud-sdk-go-v2/kafka-quotas/v1"
	ksql "github.com/confluentinc/ccloud-sdk-go-v2/ksql/v2"
	mds "github.com/confluentinc/ccloud-sdk-go-v2/mds/v2"
	netpl "github.com/confluentinc/ccloud-sdk-go-v2/networking-privatelink/v1"
	net "github.com/confluentinc/ccloud-sdk-go-v2/networking/v1"
	org "github.com/confluentinc/ccloud-sdk-go-v2/org/v2"
	srcm "github.com/confluentinc/ccloud-sdk-go-v2/srcm/v2"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

const (
	terraformProviderUserAgent = "terraform-provider-confluent"
)

const (
	paramApiVersion  = "api_version"
	paramCloud       = "cloud"
	paramRegion      = "region"
	paramEnvironment = "environment"
	paramId          = "id"
	paramDisplayName = "display_name"
	paramName        = "name"
	paramDescription = "description"
	paramKind        = "kind"
	paramCsu         = "csu"
)

type Client struct {
	apiKeysClient                   *apikeys.APIClient
	byokClient                      *byok.APIClient
	iamClient                       *iam.APIClient
	iamV1Client                     *iamv1.APIClient
	ccpClient                       *ccp.APIClient
	cmkClient                       *cmk.APIClient
	connectClient                   *connect.APIClient
	fcpmClient                      *fcpm.APIClient
	netClient                       *net.APIClient
	netIpClient                     *netip.APIClient
	netPLClient                     *netpl.APIClient
	orgClient                       *org.APIClient
	ksqlClient                      *ksql.APIClient
	kafkaRestClientFactory          *KafkaRestClientFactory
	schemaRegistryRestClientFactory *SchemaRegistryRestClientFactory
	mdsClient                       *mds.APIClient
	oidcClient                      *oidc.APIClient
	quotasClient                    *quotas.APIClient
	srcmClient                      *srcm.APIClient
	userAgent                       string
	cloudApiKey                     string
	cloudApiSecret                  string
	kafkaClusterId                  string
	kafkaApiKey                     string
	kafkaApiSecret                  string
	kafkaRestEndpoint               string
	isKafkaClusterIdSet             bool
	isKafkaMetadataSet              bool
	schemaRegistryClusterId         string
	schemaRegistryApiKey            string
	schemaRegistryApiSecret         string
	schemaRegistryRestEndpoint      string
	isSchemaRegistryMetadataSet     bool
}

// Customize configs for terraform-plugin-docs
func init() {
	schema.DescriptionKind = schema.StringMarkdown

	schema.SchemaDescriptionBuilder = func(s *schema.Schema) string {
		descriptionWithDefault := s.Description
		if s.Default != nil {
			descriptionWithDefault += fmt.Sprintf(" Defaults to `%v`.", s.Default)
		}
		return strings.TrimSpace(descriptionWithDefault)
	}
}

func New(version, userAgent string) func() *schema.Provider {
	return func() *schema.Provider {
		provider := &schema.Provider{
			Schema: map[string]*schema.Schema{
				"cloud_api_key": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("CONFLUENT_CLOUD_API_KEY", ""),
					Description: "The Confluent Cloud API Key.",
				},
				"cloud_api_secret": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("CONFLUENT_CLOUD_API_SECRET", ""),
					Description: "The Confluent Cloud API Secret.",
				},
				"kafka_id": {
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("KAFKA_ID", ""),
					Description: "The Kafka Cluster ID.",
				},
				"kafka_api_key": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("KAFKA_API_KEY", ""),
					Description: "The Kafka Cluster API Key.",
				},
				"kafka_api_secret": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("KAFKA_API_SECRET", ""),
					Description: "The Kafka Cluster API Secret.",
				},
				"kafka_rest_endpoint": {
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("KAFKA_REST_ENDPOINT", ""),
					Description: "The Kafka Cluster REST Endpoint.",
				},
				"schema_registry_id": {
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("SCHEMA_REGISTRY_ID", ""),
					Description: "The Schema Registry Cluster ID.",
				},
				"schema_registry_api_key": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("SCHEMA_REGISTRY_API_KEY", ""),
					Description: "The Schema Registry Cluster API Key.",
				},
				"schema_registry_api_secret": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("SCHEMA_REGISTRY_API_SECRET", ""),
					Description: "The Schema Registry Cluster API Secret.",
				},
				"schema_registry_rest_endpoint": {
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("SCHEMA_REGISTRY_REST_ENDPOINT", ""),
					Description: "The Schema Registry Cluster REST Endpoint.",
				},
				"endpoint": {
					Type:        schema.TypeString,
					Optional:    true,
					Default:     "https://api.confluent.cloud",
					Description: "The base endpoint of Confluent Cloud API.",
				},
				"max_retries": {
					Type:         schema.TypeInt,
					Optional:     true,
					DefaultFunc:  schema.EnvDefaultFunc("TF_PROVIDER_CONFLUENT_MAX_RETRIES", 4),
					ValidateFunc: validation.IntAtLeast(4),
					Description:  "Maximum number of retries of HTTP client. Defaults to 4.",
				},
			},
			DataSourcesMap: map[string]*schema.Resource{
				"confluent_kafka_cluster":                      kafkaDataSource(),
				"confluent_kafka_topic":                        kafkaTopicDataSource(),
				"confluent_environment":                        environmentDataSource(),
				"confluent_environments":                       environmentsDataSource(),
				"confluent_ksql_cluster":                       ksqlDataSource(),
				"confluent_flink_compute_pool":                 computePoolDataSource(),
				"confluent_identity_pool":                      identityPoolDataSource(),
				"confluent_identity_provider":                  identityProviderDataSource(),
				"confluent_ip_addresses":                       ipAddressesDataSource(),
				"confluent_kafka_client_quota":                 kafkaClientQuotaDataSource(),
				"confluent_network":                            networkDataSource(),
				"confluent_organization":                       organizationDataSource(),
				"confluent_peering":                            peeringDataSource(),
				"confluent_transit_gateway_attachment":         transitGatewayAttachmentDataSource(),
				"confluent_private_link_access":                privateLinkAccessDataSource(),
				"confluent_private_link_attachment":            privateLinkAttachmentDataSource(),
				"confluent_private_link_attachment_connection": privateLinkAttachmentConnectionDataSource(),
				"confluent_role_binding":                       roleBindingDataSource(),
				"confluent_schema":                             schemaDataSource(),
				"confluent_schemas":                            schemasDataSource(),
				"confluent_users":                              usersDataSource(),
				"confluent_service_account":                    serviceAccountDataSource(),
				"confluent_schema_registry_cluster":            schemaRegistryClusterDataSource(),
				"confluent_schema_registry_clusters":           schemaRegistryClustersDataSource(),
				"confluent_schema_registry_region":             schemaRegistryRegionDataSource(),
				"confluent_subject_mode":                       subjectModeDataSource(),
				"confluent_subject_config":                     subjectConfigDataSource(),
				"confluent_schema_registry_cluster_config":     schemaRegistryClusterConfigDataSource(),
				"confluent_schema_registry_cluster_mode":       schemaRegistryClusterModeDataSource(),
				"confluent_user":                               userDataSource(),
				"confluent_invitation":                         invitationDataSource(),
				"confluent_byok_key":                           byokDataSource(),
				"confluent_network_link_endpoint":              networkLinkEndpointDataSource(),
				"confluent_network_link_service":               networkLinkServiceDataSource(),
				"confluent_tag":                                tagDataSource(),
				"confluent_tag_binding":                        tagBindingDataSource(),
				"confluent_business_metadata":                  businessMetadataDataSource(),
				"confluent_business_metadata_binding":          businessMetadataBindingDataSource(),
			},
			ResourcesMap: map[string]*schema.Resource{
				"confluent_api_key":                            apiKeyResource(),
				"confluent_byok_key":                           byokResource(),
				"confluent_cluster_link":                       clusterLinkResource(),
				"confluent_kafka_cluster":                      kafkaResource(),
				"confluent_kafka_cluster_config":               kafkaConfigResource(),
				"confluent_environment":                        environmentResource(),
				"confluent_identity_pool":                      identityPoolResource(),
				"confluent_identity_provider":                  identityProviderResource(),
				"confluent_kafka_client_quota":                 kafkaClientQuotaResource(),
				"confluent_ksql_cluster":                       ksqlResource(),
				"confluent_flink_compute_pool":                 computePoolResource(),
				"confluent_connector":                          connectorResource(),
				"confluent_custom_connector_plugin":            customConnectorPluginResource(),
				"confluent_service_account":                    serviceAccountResource(),
				"confluent_kafka_topic":                        kafkaTopicResource(),
				"confluent_kafka_mirror_topic":                 kafkaMirrorTopicResource(),
				"confluent_kafka_acl":                          kafkaAclResource(),
				"confluent_network":                            networkResource(),
				"confluent_peering":                            peeringResource(),
				"confluent_private_link_access":                privateLinkAccessResource(),
				"confluent_private_link_attachment":            privateLinkAttachmentResource(),
				"confluent_private_link_attachment_connection": privateLinkAttachmentConnectionResource(),
				"confluent_role_binding":                       roleBindingResource(),
				"confluent_schema_registry_cluster":            schemaRegistryClusterResource(),
				"confluent_schema":                             schemaResource(),
				"confluent_schema_exporter":                    schemaExporterResource(),
				"confluent_subject_mode":                       subjectModeResource(),
				"confluent_subject_config":                     subjectConfigResource(),
				"confluent_schema_registry_cluster_mode":       schemaRegistryClusterModeResource(),
				"confluent_schema_registry_cluster_config":     schemaRegistryClusterConfigResource(),
				"confluent_transit_gateway_attachment":         transitGatewayAttachmentResource(),
				"confluent_invitation":                         invitationResource(),
				"confluent_network_link_endpoint":              networkLinkEndpointResource(),
				"confluent_network_link_service":               networkLinkServiceResource(),
				"confluent_tf_importer":                        tfImporterResource(),
				"confluent_tag":                                tagResource(),
				"confluent_tag_binding":                        tagBindingResource(),
				"confluent_business_metadata":                  businessMetadataResource(),
				"confluent_business_metadata_binding":          businessMetadataBindingResource(),
			},
		}

		provider.ConfigureContextFunc = func(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
			return providerConfigure(ctx, d, provider, version, userAgent)
		}

		return provider
	}
}

// https://github.com/hashicorp/terraform-plugin-sdk/issues/155#issuecomment-489699737
////  alternative - https://github.com/hashicorp/terraform-plugin-sdk/issues/248#issuecomment-725013327
func environmentSchema() *schema.Schema {
	return &schema.Schema{
		Type: schema.TypeList,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				paramId: {
					Type:        schema.TypeString,
					Required:    true,
					ForceNew:    true,
					Description: "The unique identifier for the environment.",
				},
			},
		},
		Required:    true,
		MinItems:    1,
		MaxItems:    1,
		ForceNew:    true,
		Description: "Environment objects represent an isolated namespace for your Confluent resources for organizational purposes.",
	}
}

// https://github.com/hashicorp/terraform-plugin-sdk/issues/155#issuecomment-489699737
////  alternative - https://github.com/hashicorp/terraform-plugin-sdk/issues/248#issuecomment-725013327
func environmentDataSourceSchema() *schema.Schema {
	return &schema.Schema{
		Type: schema.TypeList,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				paramId: {
					Type:     schema.TypeString,
					Required: true,
				},
			},
		},
		Required: true,
		MaxItems: 1,
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData, p *schema.Provider, providerVersion, additionalUserAgent string) (interface{}, diag.Diagnostics) {
	tflog.Info(ctx, "Initializing Terraform Provider for Confluent Cloud")
	endpoint := d.Get("endpoint").(string)
	cloudApiKey := d.Get("cloud_api_key").(string)
	cloudApiSecret := d.Get("cloud_api_secret").(string)
	kafkaClusterId := d.Get("kafka_id").(string)
	kafkaApiKey := d.Get("kafka_api_key").(string)
	kafkaApiSecret := d.Get("kafka_api_secret").(string)
	kafkaRestEndpoint := d.Get("kafka_rest_endpoint").(string)
	schemaRegistryClusterId := d.Get("schema_registry_id").(string)
	schemaRegistryApiKey := d.Get("schema_registry_api_key").(string)
	schemaRegistryApiSecret := d.Get("schema_registry_api_secret").(string)
	schemaRegistryRestEndpoint := d.Get("schema_registry_rest_endpoint").(string)
	maxRetries := d.Get("max_retries").(int)

	// 3 or 4 attributes should be set or not set at the same time
	// Option #2: (kafka_api_key, kafka_api_secret, kafka_rest_endpoint)
	// Option #3 (primary): (kafka_api_key, kafka_api_secret, kafka_rest_endpoint, kafka_id)
	allKafkaAttributesAreSet := (kafkaApiKey != "") && (kafkaApiSecret != "") && (kafkaRestEndpoint != "")
	allKafkaAttributesAreNotSet := (kafkaApiKey == "") && (kafkaApiSecret == "") && (kafkaRestEndpoint == "")
	justOneOrTwoKafkaAttributesAreSet := !(allKafkaAttributesAreSet || allKafkaAttributesAreNotSet)
	if justOneOrTwoKafkaAttributesAreSet {
		return nil, diag.Errorf("(kafka_api_key, kafka_api_secret, kafka_rest_endpoint) or (kafka_api_key, kafka_api_secret, kafka_rest_endpoint, kafka_id) attributes should be set or not set in the provider block at the same time")
	}

	// All 4 attributes should be set or not set at the same time
	allSchemaRegistryAttributesAreSet := (schemaRegistryApiKey != "") && (schemaRegistryApiSecret != "") && (schemaRegistryRestEndpoint != "") && (schemaRegistryClusterId != "")
	allSchemaRegistryAttributesAreNotSet := (schemaRegistryApiKey == "") && (schemaRegistryApiSecret == "") && (schemaRegistryRestEndpoint == "") && (schemaRegistryClusterId == "")
	justSubsetOfSchemaRegistryAttributesAreSet := !(allSchemaRegistryAttributesAreSet || allSchemaRegistryAttributesAreNotSet)
	if justSubsetOfSchemaRegistryAttributesAreSet {
		return nil, diag.Errorf("All 4 schema_registry_api_key, schema_registry_api_secret, schema_registry_rest_endpoint, schema_registry_id attributes should be set or not set in the provider block at the same time")
	}

	userAgent := p.UserAgent(terraformProviderUserAgent, fmt.Sprintf("%s (https://confluent.cloud; support@confluent.io)", providerVersion))
	if additionalUserAgent != "" {
		userAgent = fmt.Sprintf("%s %s", additionalUserAgent, userAgent)
	}

	apiKeysCfg := apikeys.NewConfiguration()
	byokCfg := byok.NewConfiguration()
	ccpCfg := ccp.NewConfiguration()
	cmkCfg := cmk.NewConfiguration()
	connectCfg := connect.NewConfiguration()
	fcpmCfg := fcpm.NewConfiguration()
	iamCfg := iam.NewConfiguration()
	iamV1Cfg := iamv1.NewConfiguration()
	mdsCfg := mds.NewConfiguration()
	netCfg := net.NewConfiguration()
	netIpCfg := netip.NewConfiguration()
	netPLCfg := netpl.NewConfiguration()
	oidcCfg := oidc.NewConfiguration()
	orgCfg := org.NewConfiguration()
	srcmCfg := srcm.NewConfiguration()
	ksqlCfg := ksql.NewConfiguration()
	quotasCfg := quotas.NewConfiguration()

	apiKeysCfg.Servers[0].URL = endpoint
	byokCfg.Servers[0].URL = endpoint
	ccpCfg.Servers[0].URL = endpoint
	cmkCfg.Servers[0].URL = endpoint
	connectCfg.Servers[0].URL = endpoint
	fcpmCfg.Servers[0].URL = endpoint
	iamCfg.Servers[0].URL = endpoint
	iamV1Cfg.Servers[0].URL = endpoint
	mdsCfg.Servers[0].URL = endpoint
	netCfg.Servers[0].URL = endpoint
	netIpCfg.Servers[0].URL = endpoint
	netPLCfg.Servers[0].URL = endpoint
	oidcCfg.Servers[0].URL = endpoint
	orgCfg.Servers[0].URL = endpoint
	srcmCfg.Servers[0].URL = endpoint
	ksqlCfg.Servers[0].URL = endpoint
	quotasCfg.Servers[0].URL = endpoint

	apiKeysCfg.UserAgent = userAgent
	byokCfg.UserAgent = userAgent
	ccpCfg.UserAgent = userAgent
	cmkCfg.UserAgent = userAgent
	connectCfg.UserAgent = userAgent
	fcpmCfg.UserAgent = userAgent
	iamCfg.UserAgent = userAgent
	iamV1Cfg.UserAgent = userAgent
	mdsCfg.UserAgent = userAgent
	netCfg.UserAgent = userAgent
	netIpCfg.UserAgent = userAgent
	netPLCfg.UserAgent = userAgent
	oidcCfg.UserAgent = userAgent
	orgCfg.UserAgent = userAgent
	srcmCfg.UserAgent = userAgent
	ksqlCfg.UserAgent = userAgent
	quotasCfg.UserAgent = userAgent

	var kafkaRestClientFactory *KafkaRestClientFactory
	var schemaRegistryRestClientFactory *SchemaRegistryRestClientFactory

	kafkaRestClientFactory = &KafkaRestClientFactory{userAgent: userAgent, maxRetries: &maxRetries}
	schemaRegistryRestClientFactory = &SchemaRegistryRestClientFactory{userAgent: userAgent, maxRetries: &maxRetries}

	apiKeysCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	byokCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	ccpCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	cmkCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	connectCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	fcpmCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	iamCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	iamV1Cfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	mdsCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	netCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	netIpCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	netPLCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	oidcCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	orgCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	srcmCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	ksqlCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()
	quotasCfg.HTTPClient = NewRetryableClientFactory(WithMaxRetries(maxRetries)).CreateRetryableClient()

	client := Client{
		apiKeysClient:                   apikeys.NewAPIClient(apiKeysCfg),
		byokClient:                      byok.NewAPIClient(byokCfg),
		ccpClient:                       ccp.NewAPIClient(ccpCfg),
		cmkClient:                       cmk.NewAPIClient(cmkCfg),
		connectClient:                   connect.NewAPIClient(connectCfg),
		fcpmClient:                      fcpm.NewAPIClient(fcpmCfg),
		iamClient:                       iam.NewAPIClient(iamCfg),
		iamV1Client:                     iamv1.NewAPIClient(iamV1Cfg),
		netClient:                       net.NewAPIClient(netCfg),
		netIpClient:                     netip.NewAPIClient(netIpCfg),
		netPLClient:                     netpl.NewAPIClient(netPLCfg),
		oidcClient:                      oidc.NewAPIClient(oidcCfg),
		orgClient:                       org.NewAPIClient(orgCfg),
		srcmClient:                      srcm.NewAPIClient(srcmCfg),
		ksqlClient:                      ksql.NewAPIClient(ksqlCfg),
		kafkaRestClientFactory:          kafkaRestClientFactory,
		schemaRegistryRestClientFactory: schemaRegistryRestClientFactory,
		mdsClient:                       mds.NewAPIClient(mdsCfg),
		quotasClient:                    quotas.NewAPIClient(quotasCfg),
		userAgent:                       userAgent,
		cloudApiKey:                     cloudApiKey,
		cloudApiSecret:                  cloudApiSecret,
		kafkaClusterId:                  kafkaClusterId,
		kafkaApiKey:                     kafkaApiKey,
		kafkaApiSecret:                  kafkaApiSecret,
		kafkaRestEndpoint:               kafkaRestEndpoint,
		schemaRegistryClusterId:         schemaRegistryClusterId,
		schemaRegistryApiKey:            schemaRegistryApiKey,
		schemaRegistryApiSecret:         schemaRegistryApiSecret,
		schemaRegistryRestEndpoint:      schemaRegistryRestEndpoint,
		// For simplicity, treat 3 (for Kafka) and 4 (for SR) variables as a "single" one
		isKafkaMetadataSet:          allKafkaAttributesAreSet,
		isKafkaClusterIdSet:         kafkaClusterId != "",
		isSchemaRegistryMetadataSet: allSchemaRegistryAttributesAreSet,
	}

	return &client, nil
}
