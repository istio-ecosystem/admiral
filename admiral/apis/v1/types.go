package v1

const (
	Admiral = "Admiral"
	Intuit  = "intuit"
)

type AdmiralConfig struct {
	IdpsConfig         IdpsConfig         `yaml:"idps,omitempty"`
	IgnoreIdentityList IgnoreIdentityList `yaml:"ignoreIdentityList,omitempty"`
	WorkloadDatabase   DynamoDB           `yaml:"workloadDynamoDB,omitempty"`
}

type IgnoreIdentityList struct {
	StateCheckerPeriodInSeconds int      `yaml:"stateCheckerPeriodInSeconds,omitempty"`
	DynamoDB                    DynamoDB `yaml:"dynamoDB,omitempty"`
}

type DynamoDB struct {
	TableName          string `yaml:"tableName,omitempty"`
	Region             string `yaml:"region,omitempty"`
	Role               string `yaml:"role,omitempty"`
	ClusterEnvironment string `yaml:"clusterEnvironment,omitempty"`
}

type IdpsConfig struct {
	ApiKeyId               string `yaml:"api-key-id,omitempty"`
	ApiSecretKey           string `yaml:"api-secret-key,omitempty"`
	ApiEndPoint            string `yaml:"api-endpoint,omitempty"`
	MgmtSecretKey          string `yaml:"mgmt-api-secret-key,omitempty"`
	MgmtEndpoint           string `yaml:"mgmt-endpoint,omitempty"`
	MgmtTempCredExpiry     int32  `yaml:"mgmt-temp-cred-expiry,omitempty"`
	PolicyId               string `yaml:"policy-id,omitempty"`
	ExpiryRequest          int32  `yaml:"temporary-credentials-expiry-requested-mins,omitempty"`
	KubeConfigSecretFolder string `yaml:"kubeconfig-secret-folder,omitempty"`
}
