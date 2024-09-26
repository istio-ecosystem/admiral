package admiral

import (
	"context"
	"fmt"
	"github.com/istio-ecosystem/admiral/admiral/pkg/client/loader"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/test"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	v2 "k8s.io/api/batch/v1"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"reflect"
	"sync"
	"testing"
)

func getJob(namespace string, annotations map[string]string, labels map[string]string) *v2.Job {
	job := &v2.Job{}
	job.Namespace = namespace
	spec := v2.JobSpec{
		Template: coreV1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Annotations: annotations,
				Labels:      labels,
			},
		},
	}
	job.Spec = spec
	return job
}

func TestJobController_Added(t *testing.T) {
	common.ResetSync()
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
			DeploymentAnnotation:    "sidecar.istio.io/inject",
			AdmiralIgnoreLabel:      "admiral-ignore",
		},
	}
	common.InitializeConfig(admiralParams)
	ctx := context.Background()
	ctx = context.WithValue(ctx, "clusterId", "test-cluster-k8s")
	//Jobs with the correct label are added to the cache
	mdh := test.MockClientDiscoveryHandler{}
	cache := jobCache{
		cache: map[string]*JobEntry{},
		mutex: &sync.Mutex{},
	}
	jobController := JobController{
		JobHandler: &mdh,
		Cache:      &cache,
	}
	job := getJob("job-ns", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "job", "istio-injected": "true"})
	jobWithBadLabels := getJob("jobWithBadLabels-ns", map[string]string{"admiral.io/env": "dev"}, map[string]string{"identity": "jobWithBadLabels", "random-label": "true"})
	jobWithIgnoreLabels := getJob("jobWithIgnoreLabels-ns", map[string]string{"sidecar.istio.io/inject": "true", "admiral.io/env": "dev"}, map[string]string{"identity": "jobWithIgnoreLabels", "istio-injected": "true", "admiral-ignore": "true"})
	jobWithIgnoreAnnotations := getJob("jobWithIgnoreAnnotations-ns", map[string]string{"admiral.io/ignore": "true"}, map[string]string{"identity": "jobWithIgnoreAnnotations"})
	jobWithIgnoreAnnotations.Annotations = map[string]string{"admiral.io/ignore": "true"}

	testCases := []struct {
		name                  string
		job                   *v2.Job
		expectedJob           *common.K8sObject
		id                    string
		expectedCacheContains bool
	}{
		{
			name:                  "Expects job to be added to the cache when the correct label is present",
			job:                   job,
			expectedJob:           getK8sObjectFromJob(job),
			id:                    "job",
			expectedCacheContains: true,
		},
		{
			name:                  "Expects job to not be added to the cache when the correct label is not present",
			job:                   jobWithBadLabels,
			expectedJob:           nil,
			id:                    "jobWithBadLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored job identified by label to not be added to the cache",
			job:                   jobWithIgnoreLabels,
			expectedJob:           nil,
			id:                    "jobWithIgnoreLabels",
			expectedCacheContains: false,
		},
		{
			name:                  "Expects ignored job identified by job annotation to not be added to the cache",
			job:                   jobWithIgnoreAnnotations,
			expectedJob:           nil,
			id:                    "jobWithIgnoreAnnotations",
			expectedCacheContains: false,
		},
	}
	ns := coreV1.Namespace{}
	ns.Name = "test-ns"
	ns.Annotations = map[string]string{"admiral.io/ignore": "true"}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "Expects ignored job identified by label to be removed from the cache" {
				job.Spec.Template.Labels["admiral-ignore"] = "true"
			}
			jobController.Added(ctx, c.job)
			jobClusterEntry := jobController.Cache.cache[c.id]
			var jobsMap map[string]*common.K8sObject = nil
			if jobClusterEntry != nil {
				jobsMap = jobClusterEntry.Jobs
			}
			var jobObj *common.K8sObject = nil
			if jobsMap != nil && len(jobsMap) > 0 {
				jobObj = jobsMap[c.job.Namespace]
			}
			if !reflect.DeepEqual(c.expectedJob, jobObj) {
				t.Errorf("Expected rollout %+v but got %+v", c.expectedJob, jobObj)
			}
		})
	}
}

func TestJobControlle_DoesGenerationMatch(t *testing.T) {
	dc := JobController{}

	admiralParams := common.AdmiralParams{}

	testCases := []struct {
		name                  string
		jobNew                interface{}
		jobOld                interface{}
		enableGenerationCheck bool
		expectedValue         bool
		expectedError         error
	}{
		{
			name: "Given context, new job and old job object " +
				"When new job is not of type *v1.Job " +
				"Then func should return an error",
			jobNew:                struct{}{},
			jobOld:                struct{}{},
			enableGenerationCheck: true,
			expectedError:         fmt.Errorf("type assertion failed, {} is not of type *Job"),
		},
		{
			name: "Given context, new job and old job object " +
				"When old job is not of type *v1.Job " +
				"Then func should return an error",
			jobNew:                &v2.Job{},
			jobOld:                struct{}{},
			enableGenerationCheck: true,
			expectedError:         fmt.Errorf("type assertion failed, {} is not of type *Job"),
		},
		{
			name: "Given context, new job and old job object " +
				"When job generation check is enabled but the generation does not match " +
				"Then func should return false ",
			jobNew: &v2.Job{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			jobOld: &v2.Job{
				ObjectMeta: v1.ObjectMeta{
					Generation: 1,
				},
			},
			enableGenerationCheck: true,
			expectedError:         nil,
		},
		{
			name: "Given context, new job and old job object " +
				"When job generation check is disabled " +
				"Then func should return false ",
			jobNew: &v2.Job{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			jobOld: &v2.Job{
				ObjectMeta: v1.ObjectMeta{
					Generation: 1,
				},
			},
			expectedError: nil,
		},
		{
			name: "Given context, new job and old job object " +
				"When job generation check is enabled and the old and new job generation is equal " +
				"Then func should just return true",
			jobNew: &v2.Job{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			jobOld: &v2.Job{
				ObjectMeta: v1.ObjectMeta{
					Generation: 2,
				},
			},
			enableGenerationCheck: true,
			expectedError:         nil,
			expectedValue:         true,
		},
	}

	ctxLogger := log.WithFields(log.Fields{
		"txId": "abc",
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			admiralParams.EnableGenerationCheck = tc.enableGenerationCheck
			common.ResetSync()
			common.InitializeConfig(admiralParams)
			actual, err := dc.DoesGenerationMatch(ctxLogger, tc.jobNew, tc.jobOld)
			if !ErrorEqualOrSimilar(err, tc.expectedError) {
				t.Errorf("expected: %v, got: %v", tc.expectedError, err)
			}
			if err == nil {
				if tc.expectedValue != actual {
					t.Errorf("expected: %v, got: %v", tc.expectedValue, actual)
				}
			}
		})
	}

}

func TestNewJobController(t *testing.T) {
	config, err := clientcmd.BuildConfigFromFlags("", "../../test/resources/admins@fake-cluster.k8s.local")
	if err != nil {
		t.Errorf("%v", err)
	}
	stop := make(chan struct{})
	jobHandler := test.MockClientDiscoveryHandler{}

	jobCon, _ := NewJobController(stop, &jobHandler, config, 0, loader.GetFakeClientLoader())

	if jobCon == nil {
		t.Errorf("Job controller should not be nil")
	}
}

func TestJobUpdateProcessItemStatus(t *testing.T) {
	common.ResetSync()
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(admiralParams)
	jobInCache := getJob("namespace-1", map[string]string{}, map[string]string{"identity": "job1", "env": "prd"})
	jobInCache.Name = "job1"
	jobInCache2 := getJob("namespace-2", map[string]string{}, map[string]string{"identity": "job2", "env": "prd"})
	jobInCache2.Name = "job2"
	jobNotInCache := getJob("namespace-3", map[string]string{}, map[string]string{"identity": "job3", "env": "prd"})
	jobNotInCache.Name = "job3"
	var (
		serviceAccount = &coreV1.ServiceAccount{}
	)

	// Populating the job Cache
	jobCache := &jobCache{
		cache: make(map[string]*JobEntry),
		mutex: &sync.Mutex{},
	}

	jobController := &JobController{
		Cache: jobCache,
	}

	jobCache.Put(getK8sObjectFromJob(jobInCache))
	jobController.UpdateProcessItemStatus(jobInCache, common.Processed)
	jobCache.Put(getK8sObjectFromJob(jobInCache2))

	testCases := []struct {
		name           string
		obj            interface{}
		statusToSet    string
		expectedErr    error
		expectedStatus string
	}{
		{
			name: "Given job cache has a valid job in its cache, " +
				"And is processed" +
				"Then, the status for the valid job should be updated to processed",
			obj:            jobInCache,
			statusToSet:    common.Processed,
			expectedErr:    nil,
			expectedStatus: common.Processed,
		},
		{
			name: "Given job cache has a valid job in its cache, " +
				"And is processed" +
				"Then, the status for the valid job should be updated to not processed",
			obj:            jobInCache2,
			statusToSet:    common.NotProcessed,
			expectedErr:    nil,
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given job cache does not has a valid job in its cache, " +
				"Then, the status for the valid job should be not processed, " +
				"And an error should be returned with the job not found message",
			obj:            jobNotInCache,
			statusToSet:    common.NotProcessed,
			expectedErr:    fmt.Errorf(LogCacheFormat, "UpdateStatus", "Job", jobNotInCache.Name, jobNotInCache.Namespace, "", "nothing to update, job not found in cache"),
			expectedStatus: common.NotProcessed,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:            serviceAccount,
			expectedErr:    fmt.Errorf("type assertion failed"),
			expectedStatus: common.NotProcessed,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			err := jobController.UpdateProcessItemStatus(c.obj, c.statusToSet)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			status, _ := jobController.GetProcessItemStatus(c.obj)
			assert.Equal(t, c.expectedStatus, status)
		})
	}
}

func TestJobGetProcessItemStatus(t *testing.T) {
	common.ResetSync()
	admiralParams := common.AdmiralParams{
		LabelSet: &common.LabelSet{
			WorkloadIdentityKey:     "identity",
			EnvKey:                  "admiral.io/env",
			AdmiralCRDIdentityLabel: "identity",
		},
	}
	common.InitializeConfig(admiralParams)
	var (
		serviceAccount = &coreV1.ServiceAccount{}
	)
	jobInCache := getJob("namespace-1", map[string]string{}, map[string]string{"identity": "job1"})
	jobInCache.Name = "debug-1"
	jobNotInCache := getJob("namespace-2", map[string]string{}, map[string]string{"identity": "job2"})
	jobNotInCache.Name = "debug-2"

	// Populating the job Cache
	jobCache := &jobCache{
		cache: make(map[string]*JobEntry),
		mutex: &sync.Mutex{},
	}

	jobController := &JobController{
		Cache: jobCache,
	}

	jobCache.Put(getK8sObjectFromJob(jobInCache))
	jobCache.UpdateJobProcessStatus(jobInCache, common.Processed)

	testCases := []struct {
		name           string
		obj            interface{}
		expectedErr    error
		expectedResult string
	}{
		{
			name: "Given job cache has a valid job in its cache, " +
				"And is processed" +
				"Then, we should be able to get the status as processed",
			obj:            jobInCache,
			expectedResult: common.Processed,
		},
		{
			name: "Given job cache does not has a valid job in its cache, " +
				"Then, the status for the valid job should not be updated",
			obj:            jobNotInCache,
			expectedResult: common.NotProcessed,
		},
		{
			name: "Given ServiceAccount is passed to the function, " +
				"Then, the function should not panic, " +
				"And return an error",
			obj:            serviceAccount,
			expectedErr:    fmt.Errorf("type assertion failed"),
			expectedResult: common.NotProcessed,
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			res, err := jobController.GetProcessItemStatus(c.obj)
			if !ErrorEqualOrSimilar(err, c.expectedErr) {
				t.Errorf("expected: %v, got: %v", c.expectedErr, err)
			}
			assert.Equal(t, c.expectedResult, res)
		})
	}
}

func TestJobLogValueOfAdmiralIoIgnore(t *testing.T) {
	// Test case 1: obj is not a Job object
	d := &JobController{}
	d.LogValueOfAdmiralIoIgnore("not a job")
	// No error should occur

	// Test case 2: K8sClient is nil
	d = &JobController{}
	d.LogValueOfAdmiralIoIgnore(&v2.Job{})
	// No error should occur

	d.LogValueOfAdmiralIoIgnore(&v2.Job{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"}})
	// No error should occur

	// Test case 3: AdmiralIgnoreAnnotation is set in Job object
	d = &JobController{}
	job := &v2.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Annotations: map[string]string{
				common.AdmiralIgnoreAnnotation: "true",
			},
		},
	}
	d.LogValueOfAdmiralIoIgnore(job)
	// No error should occur
}
