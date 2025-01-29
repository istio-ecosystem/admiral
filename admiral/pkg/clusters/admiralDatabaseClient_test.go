package clusters

import (
	"errors"
	"fmt"
	v1 "github.com/istio-ecosystem/admiral/admiral/apis/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"testing"
	"time"
)

/*
import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/istio-ecosystem/admiral/admiral/apis/v1"
	"github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/model"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewAdmiralDatabaseClient(t *testing.T) {

	var dummyDynamoClientFunc = func(role, region string) (*DynamoClient, error) {
		return nil, nil
	}

	var dummyDynamoClientFuncWithError = func(role, region string) (*DynamoClient, error) {
		return nil, fmt.Errorf("failed to initialize client")
	}

	testCases := []struct {
		name              string
		admiralConfigPath string
		dynamoClientFunc  func(role, region string) (*DynamoClient, error)
		expectedErr       error
	}{
		{
			name: "Given admiral config contains all required configurations, " +
				"When NewAdmiralDatabseClient is called, " +
				"Then it should initialize admiralDatabaseClient and not return any error",
			admiralConfigPath: "testdata/admiralDatabaseClientConfig_is_valid.yaml",
			dynamoClientFunc:  dummyDynamoClientFunc,
			expectedErr:       nil,
		},
		{
			name: "Given admiral config does not contain valid configurations, " +
				"When NewAdmiralDatabseClient is called, " +
				"Then it should not initialize admiralDatabaseClient and return error",
			admiralConfigPath: "testdata/admiralDatabaseClientConfig_is_not_valid.yaml",
			dynamoClientFunc:  dummyDynamoClientFunc,
			expectedErr:       fmt.Errorf("error unmarshalling admiral config file, err: yaml: line 20: mapping values are not allowed in this context"),
		},
		{
			name: "Given admiral config contains all required configurations but fails to create dynamodb client, " +
				"When NewAdmiralDatabseClient is called, " +
				"Then it should not initialize admiralDatabaseClient and return error",
			admiralConfigPath: "testdata/admiralDatabaseClientConfig_is_valid.yaml",
			dynamoClientFunc:  dummyDynamoClientFuncWithError,
			expectedErr:       fmt.Errorf("unable to instantiate dynamo client, err: failed to initialize client"),
		},
		{
			name: "Given incorrect admiral config path, " +
				"When NewAdmiralDatabseClient is called, " +
				"Then it should not initialize admiralDatabaseClient and return error",
			admiralConfigPath: "testdata/admiralDatabaseClientConfig_is_invalid.yaml",
			dynamoClientFunc:  dummyDynamoClientFuncWithError,
			expectedErr:       fmt.Errorf("error reading admiral config file, err: open testdata/admiralDatabaseClientConfig_is_invalid.yaml: no such file or directory"),
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewAdmiralDatabaseClient(c.admiralConfigPath, c.dynamoClientFunc)
			if c.expectedErr != nil {
				assert.EqualError(t, err, c.expectedErr.Error())
			} else {
				if err != c.expectedErr {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}
		})
	}
}

func TestUpdateWorkloadData(t *testing.T) {

	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})

	var DynamodbClient = DynamoClient{
		&mockDynamoDBClient{},
	}

	var databaseClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: &DynamodbClient,
	}

	var databaseClientWithNilDynamoClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: nil,
	}

	var databaseClientWithNilDatabase = WorkloadDatabaseClient{
		database:     nil,
		dynamoClient: &DynamodbClient,
	}

	var DynamodbClientWithError = DynamoClient{
		&mockDynamoDBClientWithErrors{},
	}

	var databaseClientWithError = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: &DynamodbClientWithError,
	}

	var workloadDataWithoutGTP = WorkloadData{
		AssetAlias: "custom",
		Endpoint:   "dev.custom.global",
		Env:        "dev",
		Aliases:    []string{"dev.custom.testsuffix"},
	}

	var workloadDataWithFailoverGTP = WorkloadData{
		AssetAlias: "custom",
		Endpoint:   "dev.custom.global",
		Env:        "dev",
		Aliases:    []string{"dev.custom.testsuffix"},
		LbType:     model.TrafficPolicy_FAILOVER.String(),
		TrafficDistribution: map[string]int32{
			"us-west-2": 100,
		},
	}

	var workloadDataWithTopologyGTP = WorkloadData{
		AssetAlias: "custom",
		Endpoint:   "dev.custom.global",
		Env:        "dev",
		Aliases:    []string{"dev.custom.testsuffix"},
		LbType:     model.TrafficPolicy_TOPOLOGY.String(),
	}

	testCases := []struct {
		name           string
		workloadData   WorkloadData
		databaseClient WorkloadDatabaseClient
		expectedErr    error
	}{
		{
			name: "Given workload object and no globaltrafficpolicy configuration, " +
				"When UpdateWorkloadData is called, " +
				"Then it should not return any error",
			workloadData:   workloadDataWithoutGTP,
			databaseClient: databaseClient,
			expectedErr:    nil,
		},
		{
			name: "Given workload object with failover globaltrafficpolicy configuration, " +
				"When UpdateWorkloadData is called, " +
				"Then it should not return any error",
			workloadData:   workloadDataWithFailoverGTP,
			databaseClient: databaseClient,
			expectedErr:    nil,
		},
		{
			name: "Given workload object with topology globaltrafficpolicy configuration, " +
				"When UpdateWorkloadData is called, " +
				"Then it should not return any error",
			workloadData:   workloadDataWithTopologyGTP,
			databaseClient: databaseClient,
			expectedErr:    nil,
		},
		{
			name: "Given workload object with failover globaltrafficpolicy configuration and error occurs using dynamodb client " +
				"When UpdateWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadDataWithFailoverGTP,
			databaseClient: databaseClientWithError,
			expectedErr:    fmt.Errorf("error occurred adding record to dynamodb"),
		},
		{
			name: "Given dynamodb client is nil" +
				"When UpdateWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadDataWithFailoverGTP,
			databaseClient: databaseClientWithNilDynamoClient,
			expectedErr:    fmt.Errorf("dynamoClient is not initialized"),
		},
		{
			name: "Given database is nil" +
				"When UpdateWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadDataWithFailoverGTP,
			databaseClient: databaseClientWithNilDatabase,
			expectedErr:    fmt.Errorf("database is not initialized"),
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := c.databaseClient.Update(c.workloadData, ctxLogger)
			if c.expectedErr != nil {
				assert.EqualError(t, err, c.expectedErr.Error())
			} else {
				if err != c.expectedErr {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}
		})
	}
}

type mockDynamoDBClientForGet struct {
	dynamodbiface.DynamoDBAPI
}

func (m *mockDynamoDBClientForGet) Query(queryInput *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
	switch *queryInput.TableName {
	case workloadDataTableName:
		return &dynamodb.QueryOutput{
			Items: []map[string]*dynamodb.AttributeValue{
				{
					"assetAlias": {S: aws.String("intuit.test.asset")},
					"env":        {S: aws.String("testEnv")},
					"dnsPrefix":  {S: aws.String("west")},
					"lbType":     {S: aws.String("FAILOVER")},
					"trafficDistribution": {M: map[string]*dynamodb.AttributeValue{
						"us-west-2": {N: aws.String("100")},
						"us-east-2": {N: aws.String("0")},
					}},
					"gtpManagedBy": {S: aws.String("github")},
				},
			},
		}, nil
	default:
		return nil, nil
	}
}

func (m *mockDynamoDBClientForGet) Scan(input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error) {
	return nil, nil
}

func (m *mockDynamoDBClientForGet) GetItem(input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
	return nil, nil
}

func (m *mockDynamoDBClientForGet) PutItem(input *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockDynamoDBClientForGet) DeleteItem(input *dynamodb.DeleteItemInput) (*dynamodb.DeleteItemOutput, error) {
	return &dynamodb.DeleteItemOutput{}, nil
}

func TestGetWorkloadDataOnDatabaseClient(t *testing.T) {

	var DynamodbClient = DynamoClient{
		&mockDynamoDBClientForGet{},
	}

	var databaseClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: &DynamodbClient,
	}

	var databaseClientWithNilDynamoClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: nil,
	}

	var databaseClientWithNilDatabase = WorkloadDatabaseClient{
		database:     nil,
		dynamoClient: &DynamodbClient,
	}

	testCases := []struct {
		name                  string
		identity              string
		env                   string
		workloadDataItems     []WorkloadData
		databaseClient        WorkloadDatabaseClient
		expectedErr           error
		expectedNumberOfItems int
	}{
		{
			name: "Given identity and env parameter, " +
				"And, client is able to query the given table, " +
				"And, one of the items has identity set to 'intuit.test.asset'," +
				"When Get is called, " +
				"Then, it should return expected items without any error",
			identity:              "intuit.test.asset",
			env:                   "testEnv",
			databaseClient:        databaseClient,
			expectedNumberOfItems: 1,
			expectedErr:           nil,
		},
		{
			name: "Given dynamodb client is nil" +
				"When Get is called on databaseClient, " +
				"Then it should return error",
			databaseClient: databaseClientWithNilDynamoClient,
			expectedErr:    fmt.Errorf("dynamoClient is not initialized"),
		},
		{
			name: "Given database is nil" +
				"When Get is called on databaseClient, " +
				"Then it should return error",
			databaseClient: databaseClientWithNilDatabase,
			expectedErr:    fmt.Errorf("database is not initialized"),
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.databaseClient.Get(c.identity, c.env)
			if c.expectedErr != nil {
				assert.EqualError(t, err, c.expectedErr.Error())
			} else {
				if err != c.expectedErr {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}
		})
	}
}

func TestDeleteWorkloadData(t *testing.T) {

	ctxLogger := logrus.WithFields(logrus.Fields{"txId": "abc"})

	var DynamodbClient = DynamoClient{
		&mockDynamoDBClient{},
	}

	var databaseClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: &DynamodbClient,
	}

	var DynamodbClientWithError = DynamoClient{
		&mockDynamoDBClientWithErrors{},
	}

	var databaseClientWithNilDynamoClient = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: nil,
	}

	var databaseClientWithNilDatabase = WorkloadDatabaseClient{
		database:     nil,
		dynamoClient: &DynamodbClient,
	}

	var databaseClientWithError = WorkloadDatabaseClient{
		database: &v1.DynamoDB{
			TableName: "test-table",
		},
		dynamoClient: &DynamodbClientWithError,
	}

	var workloadData = WorkloadData{
		Endpoint:   "dev.custom.global",
		AssetAlias: "custom",
	}

	testCases := []struct {
		name           string
		workloadData   WorkloadData
		databaseClient WorkloadDatabaseClient
		expectedErr    error
	}{
		{
			name: "Given workload object, " +
				"When deleteWorkloadData is called, " +
				"Then it should not return any error",
			workloadData:   workloadData,
			databaseClient: databaseClient,
			expectedErr:    nil,
		},
		{
			name: "Given worklaod object,and error occurs using dynamodb client " +
				"When deleteWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadData,
			databaseClient: databaseClientWithError,
			expectedErr:    fmt.Errorf("error occurred deleting the item"),
		},
		{
			name: "Given dynamodb client is nil" +
				"When deleteWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadData,
			databaseClient: databaseClientWithNilDynamoClient,
			expectedErr:    fmt.Errorf("dynamoClient is not initialized"),
		},
		{
			name: "Given database is nil" +
				"When deleteWorkloadData is called, " +
				"Then it should return error",
			workloadData:   workloadData,
			databaseClient: databaseClientWithNilDatabase,
			expectedErr:    fmt.Errorf("database is not initialized"),
		},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := c.databaseClient.Delete(c.workloadData, ctxLogger)
			if c.expectedErr != nil {
				assert.EqualError(t, err, c.expectedErr.Error())
			} else {
				if err != c.expectedErr {
					t.Errorf("expected error to be: %v, got: %v", c.expectedErr, err)
				}
			}
		})
	}
}
*/

func TestIsDynamicConfigChanged(t *testing.T) {
	type args struct {
		config DynamicConfigData
	}

	config := DynamicConfigData{}

	config1 := DynamicConfigData{EnableDynamicConfig: common.Admiral}

	config2 := config1
	config2.NLBEnabledIdentityList = []string{"identity1", "identity2"}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{"When empty config send then it should return false", args{config}, false},
		{"When admiral is loading/or no previous checksum present then it should return true", args{config1}, true},
		{"When admiral is loaded and no checksum mismatch then it should return false", args{config1}, false},
		{"When admiral is loaded and checksum mismatched then it should return true", args{config2}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, IsDynamicConfigChanged(tt.args.config), "IsDynamicConfigChanged(%v)", tt.args.config)
		})
	}
}

func TestUpdateSyncAdmiralConfig(t *testing.T) {
	type args struct {
		configData DynamicConfigData
	}

	configUpdated := DynamicConfigData{EnableDynamicConfig: common.Admiral}
	configUpdated.NLBEnabledClusters = []string{"cluster1", "cluster2"}
	configUpdated.NLBEnabledIdentityList = []string{"identity1", "identity2"}
	configUpdated.CLBEnabledClusters = []string{"cluster1", "cluster2"}

	expectedAdmiralConfig := common.GetAdmiralParams()
	expectedAdmiralConfig.NLBEnabledClusters = []string{"cluster1", "cluster2"}
	expectedAdmiralConfig.CLBEnabledClusters = []string{"cluster1", "cluster2"}
	expectedAdmiralConfig.NLBEnabledIdentityList = []string{"identity1", "identity2"}

	emptyConfig := DynamicConfigData{}
	expectedEmptyConfig := common.GetAdmiralParams()

	tests := []struct {
		name string
		args args
		want common.AdmiralParams
	}{
		{"EmptyConfig", args{emptyConfig}, expectedEmptyConfig},
		{"AdmiralConfigUpdate", args{configUpdated}, expectedAdmiralConfig},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			UpdateSyncAdmiralConfig(tt.args.configData)
			assert.Equal(t, tt.want, common.GetAdmiralParams())
		})
	}
}

func TestNewDynamicConfigDatabaseClient(t *testing.T) {
	var dummyDynamoClientFunc = func(role, region string) (*DynamoClient, error) {
		return nil, nil
	}

	var dummyDynamoClientFuncWithError = func(role, region string) (*DynamoClient, error) {
		return nil, fmt.Errorf("failed to initialize client")
	}

	type args struct {
		path                 string
		dynamoClientInitFunc func(role string, region string) (*DynamoClient, error)
	}

	var dynamicConfigClient = DynamicConfigDatabaseClient{}

	expectedV1AdmiralConfig := v1.AdmiralConfig{}
	dynamicConfigClient.database = &expectedV1AdmiralConfig.DynamicConfigDatabase
	dynamicConfigClient.database.TableName = common.GetAdmiralParams().DynamicConfigDynamoDBTableName

	testArgsValid := args{
		path:                 "testdata/admiralDatabaseClientConfig_is_valid.yaml",
		dynamoClientInitFunc: dummyDynamoClientFunc,
	}

	testArgsError := args{
		path:                 "testdata/admiralDatabaseClientConfig_is_valid.yaml",
		dynamoClientInitFunc: dummyDynamoClientFuncWithError,
	}

	testArgsErrorMarshalling := args{
		path:                 "testdata/admiralDatabaseClientConfig_invalid.yaml",
		dynamoClientInitFunc: dummyDynamoClientFunc,
	}

	tests := []struct {
		name    string
		args    args
		want    *DynamicConfigDatabaseClient
		wantErr error
	}{
		{"When valid config is passed then expected client to be initialize with no error", testArgsValid, &dynamicConfigClient, nil},
		{"When valid is passed then expected error", testArgsError, &dynamicConfigClient, errors.New("unable to instantiate dynamo client for DynamicConfig")},
		{"When invalid config is passed then expect error", testArgsErrorMarshalling, nil, errors.New("error unmarshalling admiral config file")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewDynamicConfigDatabaseClient(tt.args.path, tt.args.dynamoClientInitFunc)
			if tt.wantErr != nil {
				assert.Contains(t, err.Error(), tt.wantErr.Error())
			}
			assert.Equalf(t, tt.want, got, "NewDynamicConfigDatabaseClient(%v, %v)", tt.args.path, tt.args.dynamoClientInitFunc)
		})
	}
}

type DummyDynamicConfigDatabaseClient struct {
	DynamoClient *DynamoClient
	datbaase     *v1.DynamoDB
}

func (d DummyDynamicConfigDatabaseClient) Update(data interface{}, logger *log.Entry) error {
	//TODO implement me
	panic("implement me")
}

func (d DummyDynamicConfigDatabaseClient) Delete(data interface{}, logger *log.Entry) error {
	//TODO implement me
	panic("implement me")
}

func (d DummyDynamicConfigDatabaseClient) Get(env, identity string) (interface{}, error) {
	dummyDynamicConfigData := DynamicConfigData{
		EnableDynamicConfig:    common.Admiral,
		NLBEnabledClusters:     []string{"cluster1"},
		NLBEnabledIdentityList: []string{"identity1", "identity2"},
		CLBEnabledClusters:     []string{"cluster1"},
	}

	return dummyDynamicConfigData, nil
}

func TestReadAndUpdateSyncAdmiralConfig(t *testing.T) {

	var testData DummyDynamicConfigDatabaseClient

	testAdmiralParam := common.GetAdmiralParams()
	testAdmiralParam.LabelSet.GatewayApp = common.IstioIngressGatewayLabelValue
	testAdmiralParam.NLBIngressLabel = common.NLBIstioIngressGatewayLabelValue
	testAdmiralParam.NLBEnabledClusters = []string{"cluster1"}

	common.UpdateAdmiralParams(testAdmiralParam)

	rr := NewRemoteRegistry(nil, common.AdmiralParams{})

	stop := make(chan struct{})
	config := rest.Config{
		Host: "localhost",
	}

	testService := k8sv1.Service{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "clb",
			Namespace:         common.NamespaceIstioSystem,
			Generation:        0,
			CreationTimestamp: metav1.Time{},
			Labels:            map[string]string{common.App: common.IstioIngressGatewayLabelValue},
		},
		Spec: k8sv1.ServiceSpec{},
		Status: k8sv1.ServiceStatus{
			LoadBalancer: k8sv1.LoadBalancerStatus{Ingress: make([]k8sv1.LoadBalancerIngress, 0)},
			Conditions:   nil,
		},
	}

	portStatus := k8sv1.PortStatus{
		Port:     007,
		Protocol: "HTTP",
		Error:    nil,
	}

	testLoadBalancerIngress := k8sv1.LoadBalancerIngress{
		IP:       "007.007.007.007",
		Hostname: "clb.istio.com",
		IPMode:   nil,
		Ports:    make([]k8sv1.PortStatus, 0),
	}
	testLoadBalancerIngress.Ports = append(testLoadBalancerIngress.Ports, portStatus)
	testService.Status.LoadBalancer.Ingress = append(testService.Status.LoadBalancer.Ingress, testLoadBalancerIngress)

	testService1 := testService.DeepCopy()
	testService1.Name = "nlb"
	testService1.Labels[common.App] = common.NLBIstioIngressGatewayLabelValue
	testService1.Status.LoadBalancer.Ingress[0].Hostname = "nlb.istio.com"

	testService2 := testService1.DeepCopy()
	testService2.Labels[common.App] = common.NLBIstioIngressGatewayLabelValue + "TEST"
	testService2.Name = "nlb2"

	testServiceControler, _ := admiral.NewServiceController(stop, &test.MockServiceHandler{}, &config, time.Second*time.Duration(300), loader.GetFakeClientLoader())
	testServiceControler.Cache.Put(&testService)
	testServiceControler.Cache.Put(testService1)
	testServiceControler.Cache.Put(testService2)

	rr.remoteControllers["cluster1"] = &RemoteController{
		ClusterID:                        "",
		ApiServer:                        "",
		StartTime:                        time.Time{},
		GlobalTraffic:                    nil,
		DeploymentController:             nil,
		ServiceController:                testServiceControler,
		NodeController:                   nil,
		ServiceEntryController:           nil,
		DestinationRuleController:        nil,
		VirtualServiceController:         nil,
		SidecarController:                nil,
		RolloutController:                nil,
		RoutingPolicyController:          nil,
		OutlierDetectionController:       nil,
		ClientConnectionConfigController: nil,
		JobController:                    nil,
		VertexController:                 nil,
		MonoVertexController:             nil,
		stop:                             nil,
	}

	rr.DynamicConfigDatabaseClient = testData

	type args struct {
		dbClient AdmiralDatabaseManager
	}

	var testArgs = args{dbClient: testData}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{"When ReadAndUpdateSyncAdmiralConfig invoked with valid DynamoClient then expect no error", testArgs, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ReadAndUpdateSyncAdmiralConfig(rr)
			if tt.wantErr != nil {
				assert.Contains(t, err.Error(), tt.wantErr.Error(), "ReadAndUpdateSyncAdmiralConfig(). Expect error containing %s but got error = %v", tt.wantErr.Error(), err.Error())
			} else {
				assert.Nil(t, err, "ReadAndUpdateSyncAdmiralConfig(). Expect no error but got error - %s", err)
			}
		})
	}
}

func Test_getLBToProcess(t *testing.T) {
	rr := NewRemoteRegistry(nil, common.AdmiralParams{})
	rr.AdmiralCache.NLBEnabledCluster = []string{}

	type args struct {
		updatedLB []string
		cache     *[]string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{"When cache is not updated then getLBToProcess should be all updated list ",
			args{updatedLB: []string{"cluster1", "cluster2"}, cache: &[]string{}}, []string{"cluster1", "cluster2"}},
		{"When cluster is removed from update list then getLBToProcess should return removed cluster",
			args{updatedLB: []string{"cluster1", "cluster2"}, cache: &[]string{"cluster1", "cluster2", "cluster3"}}, []string{"cluster3"}},
		{"When cluster is added from update list then getLBToProcess should return added cluster",
			args{updatedLB: []string{"cluster1", "cluster2", "cluster3"}, cache: &[]string{"cluster1", "cluster2"}}, []string{"cluster3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, getLBToProcess(tt.args.updatedLB, tt.args.cache), "getLBToProcess(%v, %v)", tt.args.updatedLB, *tt.args.cache)
		})
	}
}
