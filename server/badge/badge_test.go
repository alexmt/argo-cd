package badge

import (
	"context"
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	appclientset "github.com/argoproj/argo-cd/v2/pkg/client/clientset/versioned/fake"
	"github.com/argoproj/argo-cd/v2/util/settings"

	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	argoCDSecret = corev1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: "argocd-secret", Namespace: "default"},
		Data: map[string][]byte{
			"admin.password":   []byte("test"),
			"server.secretkey": []byte("test"),
			"server.csrfkey":   []byte("12345678901234567890123456789012"),
		},
	}
	argoCDCm = corev1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "argocd-cm",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "argocd",
			},
		},
		Data: map[string]string{
			"statusbadge.enabled": "true",
		},
	}
	testApp = v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "default"},
		Status: v1alpha1.ApplicationStatus{
			Sync:   v1alpha1.SyncStatus{Status: v1alpha1.SyncStatusCodeSynced},
			Health: v1alpha1.HealthStatus{Status: health.HealthStatusHealthy},
			OperationState: &v1alpha1.OperationState{
				SyncResult: &v1alpha1.SyncOperationResult{
					Revision: "aa29b85",
				},
			},
		},
	}
	testApp2 = v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "argocd-test"},
		Status: v1alpha1.ApplicationStatus{
			Sync:   v1alpha1.SyncStatus{Status: v1alpha1.SyncStatusCodeSynced},
			Health: v1alpha1.HealthStatus{Status: health.HealthStatusHealthy},
			OperationState: &v1alpha1.OperationState{
				SyncResult: &v1alpha1.SyncOperationResult{
					Revision: "aa29b85",
				},
			},
		},
	}
	testProject = v1alpha1.AppProject{
		ObjectMeta: v1.ObjectMeta{Name: "test-project", Namespace: "default"},
		Spec:       v1alpha1.AppProjectSpec{},
	}
)

func TestHandlerFeatureIsEnabled(t *testing.T) {
	settingsMgr := settings.NewSettingsManager(context.Background(), fake.NewSimpleClientset(&argoCDCm, &argoCDSecret), "default")
	handler := NewHandler(appclientset.NewSimpleClientset(&testApp), settingsMgr, "default", []string{})
	req, err := http.NewRequest(http.MethodGet, "/api/badge?name=test-app", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	response := rr.Body.String()
	assert.Equal(t, toRGBString(Green), leftRectColorPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, toRGBString(Green), rightRectColorPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, "Healthy", leftTextPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, "Synced", rightTextPattern.FindStringSubmatch(response)[1])
	assert.NotContains(t, response, "(aa29b85)")
}

func TestHandlerFeatureProjectIsEnabled(t *testing.T) {
	projectTests := []struct {
		testApp     []*v1alpha1.Application
		response    int
		apiEndPoint string
		namespace   string
		health      string
		status      string
		healthColor color.RGBA
		statusColor color.RGBA
	}{
		{createApplications([]string{"Healthy:Synced", "Healthy:Synced"}, []string{"default", "default"}, "test"),
			http.StatusOK, "/api/badge?project=default", "test", "Healthy", "Synced", Green, Green},
		{createApplications([]string{"Healthy:Synced", "Healthy:OutOfSync"}, []string{"test-project", "test-project"}, "default"),
			http.StatusOK, "/api/badge?project=test-project", "default", "Healthy", "OutOfSync", Green, Orange},
		{createApplications([]string{"Healthy:Synced", "Degraded:Synced"}, []string{"default", "default"}, "test"),
			http.StatusOK, "/api/badge?project=default", "test", "Degraded", "Synced", Red, Green},
		{createApplications([]string{"Healthy:Synced", "Degraded:OutOfSync"}, []string{"test-project", "test-project"}, "default"),
			http.StatusOK, "/api/badge?project=test-project", "default", "Degraded", "OutOfSync", Red, Orange},
		{createApplications([]string{"Healthy:Synced", "Healthy:Synced"}, []string{"test-project", "default"}, "test"),
			http.StatusOK, "/api/badge?project=default&project=test-project", "test", "Healthy", "Synced", Green, Green},
		{createApplications([]string{"Healthy:OutOfSync", "Healthy:Synced"}, []string{"test-project", "default"}, "default"),
			http.StatusOK, "/api/badge?project=default&project=test-project", "default", "Healthy", "OutOfSync", Green, Orange},
		{createApplications([]string{"Degraded:Synced", "Healthy:Synced"}, []string{"test-project", "default"}, "test"),
			http.StatusOK, "/api/badge?project=default&project=test-project", "test", "Degraded", "Synced", Red, Green},
		{createApplications([]string{"Degraded:OutOfSync", "Healthy:OutOfSync"}, []string{"test-project", "default"}, "default"),
			http.StatusOK, "/api/badge?project=default&project=test-project", "default", "Degraded", "OutOfSync", Red, Orange},
		{createApplications([]string{"Unknown:Unknown", "Unknown:Unknown"}, []string{"test-project", "default"}, "default"),
			http.StatusOK, "/api/badge?project=", "default", "Unknown", "Unknown", Purple, Purple},
		{createApplications([]string{"Unknown:Unknown", "Unknown:Unknown"}, []string{"test-project", "default"}, "default"),
			http.StatusBadRequest, "/api/badge?project=test$project", "default", "Unknown", "Unknown", Purple, Purple},
		{createApplications([]string{"Unknown:Unknown", "Unknown:Unknown"}, []string{"test-project", "default"}, "default"),
			http.StatusOK, "/api/badge?project=unknown", "default", "Unknown", "Unknown", Purple, Purple},
		{createApplications([]string{"Unknown:Unknown", "Unknown:Unknown"}, []string{"test-project", "default"}, "default"),
			http.StatusBadRequest, "/api/badge?name=foo_bar", "default", "Unknown", "Unknown", Purple, Purple},
		{createApplications([]string{"Unknown:Unknown", "Unknown:Unknown"}, []string{"test-project", "default"}, "default"),
			http.StatusOK, "/api/badge?name=foobar", "default", "Not Found", "", Purple, Purple},
	}
	for _, tt := range projectTests {
		argoCDCm.ObjectMeta.Namespace = tt.namespace
		argoCDSecret.ObjectMeta.Namespace = tt.namespace
		settingsMgr := settings.NewSettingsManager(context.Background(), fake.NewSimpleClientset(&argoCDCm, &argoCDSecret), tt.namespace)
		handler := NewHandler(appclientset.NewSimpleClientset(&testProject, tt.testApp[0], tt.testApp[1]), settingsMgr, tt.namespace, []string{})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, tt.apiEndPoint, nil)
		assert.NoError(t, err)
		handler.ServeHTTP(rr, req)
		require.Equal(t, tt.response, rr.Result().StatusCode)
		if rr.Result().StatusCode != 400 {
			assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
			assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
			response := rr.Body.String()
			require.Greater(t, len(response), 2)
			assert.Equal(t, toRGBString(tt.healthColor), leftRectColorPattern.FindStringSubmatch(response)[1])
			assert.Equal(t, toRGBString(tt.statusColor), rightRectColorPattern.FindStringSubmatch(response)[1])
			assert.Equal(t, tt.health, leftTextPattern.FindStringSubmatch(response)[1])
			assert.Equal(t, tt.status, rightTextPattern.FindStringSubmatch(response)[1])
		}
	}
}

func TestHandlerNamespacesIsEnabled(t *testing.T) {
	t.Run("Application in allowed namespace", func(t *testing.T) {
		settingsMgr := settings.NewSettingsManager(context.Background(), fake.NewSimpleClientset(&argoCDCm, &argoCDSecret), "default")
		handler := NewHandler(appclientset.NewSimpleClientset(&testApp2), settingsMgr, "default", []string{"argocd-test"})
		req, err := http.NewRequest(http.MethodGet, "/api/badge?name=test-app&namespace=argocd-test", nil)
		assert.NoError(t, err)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
		assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

		response := rr.Body.String()
		assert.Equal(t, toRGBString(Green), leftRectColorPattern.FindStringSubmatch(response)[1])
		assert.Equal(t, toRGBString(Green), rightRectColorPattern.FindStringSubmatch(response)[1])
		assert.Equal(t, "Healthy", leftTextPattern.FindStringSubmatch(response)[1])
		assert.Equal(t, "Synced", rightTextPattern.FindStringSubmatch(response)[1])
		assert.NotContains(t, response, "(aa29b85)")
	})

	t.Run("Application in disallowed namespace", func(t *testing.T) {
		settingsMgr := settings.NewSettingsManager(context.Background(), fake.NewSimpleClientset(&argoCDCm, &argoCDSecret), "default")
		handler := NewHandler(appclientset.NewSimpleClientset(&testApp2), settingsMgr, "default", []string{"argocd-test"})
		req, err := http.NewRequest(http.MethodGet, "/api/badge?name=test-app&namespace=kube-system", nil)
		assert.NoError(t, err)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Result().StatusCode)
		response := rr.Body.String()
		assert.Equal(t, toRGBString(Purple), leftRectColorPattern.FindStringSubmatch(response)[1])
		assert.Equal(t, toRGBString(Purple), rightRectColorPattern.FindStringSubmatch(response)[1])
		assert.Equal(t, "Not Found", leftTextPattern.FindStringSubmatch(response)[1])
		assert.Equal(t, "", rightTextPattern.FindStringSubmatch(response)[1])

	})

	t.Run("Request with illegal namespace", func(t *testing.T) {
		settingsMgr := settings.NewSettingsManager(context.Background(), fake.NewSimpleClientset(&argoCDCm, &argoCDSecret), "default")
		handler := NewHandler(appclientset.NewSimpleClientset(&testApp2), settingsMgr, "default", []string{"argocd-test"})
		req, err := http.NewRequest(http.MethodGet, "/api/badge?name=test-app&namespace=kube()system", nil)
		assert.NoError(t, err)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
	})
}

func createApplicationFeatureProjectIsEnabled(healthStatus health.HealthStatusCode, syncStatus v1alpha1.SyncStatusCode, appName, projectName, namespace string) *v1alpha1.Application {
	return &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: appName, Namespace: namespace},
		Status: v1alpha1.ApplicationStatus{
			Sync:   v1alpha1.SyncStatus{Status: syncStatus},
			Health: v1alpha1.HealthStatus{Status: healthStatus},
			OperationState: &v1alpha1.OperationState{
				SyncResult: &v1alpha1.SyncOperationResult{},
			},
		},
		Spec: v1alpha1.ApplicationSpec{
			Project: projectName,
		},
	}
}

func createApplications(appCombo, projectName []string, namespace string) []*v1alpha1.Application {
	apps := make([]*v1alpha1.Application, len(appCombo))
	healthStatus := func(healthType string) health.HealthStatusCode {
		switch healthType {
		case "Healthy":
			return health.HealthStatusHealthy
		case "Degraded":
			return health.HealthStatusDegraded
		default:
			return health.HealthStatusUnknown
		}
	}
	syncStatus := func(syncType string) v1alpha1.SyncStatusCode {
		switch syncType {
		case "Synced":
			return v1alpha1.SyncStatusCodeSynced
		case "OutOfSync":
			return v1alpha1.SyncStatusCodeOutOfSync
		default:
			return v1alpha1.SyncStatusCodeUnknown
		}
	}
	for k, v := range appCombo {
		a := strings.Split(v, ":")
		healthApp := healthStatus(a[0])
		syncApp := syncStatus(a[1])
		appName := fmt.Sprintf("App %v", k)
		apps[k] = createApplicationFeatureProjectIsEnabled(healthApp, syncApp, appName, projectName[k], namespace)
	}
	return apps
}
func TestHandlerFeatureIsEnabledRevisionIsEnabled(t *testing.T) {
	settingsMgr := settings.NewSettingsManager(context.Background(), fake.NewSimpleClientset(&argoCDCm, &argoCDSecret), "default")
	handler := NewHandler(appclientset.NewSimpleClientset(&testApp), settingsMgr, "default", []string{})
	req, err := http.NewRequest(http.MethodGet, "/api/badge?name=test-app&revision=true", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	response := rr.Body.String()
	assert.Equal(t, toRGBString(Green), leftRectColorPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, toRGBString(Green), rightRectColorPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, "Healthy", leftTextPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, "Synced", rightTextPattern.FindStringSubmatch(response)[1])
	assert.Contains(t, response, "(aa29b85)")
}

func TestHandlerRevisionIsEnabledNoOperationState(t *testing.T) {
	app := testApp.DeepCopy()
	app.Status.OperationState = nil

	settingsMgr := settings.NewSettingsManager(context.Background(), fake.NewSimpleClientset(&argoCDCm, &argoCDSecret), "default")
	handler := NewHandler(appclientset.NewSimpleClientset(app), settingsMgr, "default", []string{})
	req, err := http.NewRequest(http.MethodGet, "/api/badge?name=test-app&revision=true", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	response := rr.Body.String()
	assert.Equal(t, toRGBString(Green), leftRectColorPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, toRGBString(Green), rightRectColorPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, "Healthy", leftTextPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, "Synced", rightTextPattern.FindStringSubmatch(response)[1])
	assert.NotContains(t, response, "(aa29b85)")
}

func TestHandlerRevisionIsEnabledShortCommitSHA(t *testing.T) {
	app := testApp.DeepCopy()
	app.Status.OperationState.SyncResult.Revision = "abc"

	settingsMgr := settings.NewSettingsManager(context.Background(), fake.NewSimpleClientset(&argoCDCm, &argoCDSecret), "default")
	handler := NewHandler(appclientset.NewSimpleClientset(app), settingsMgr, "default", []string{})
	req, err := http.NewRequest(http.MethodGet, "/api/badge?name=test-app&revision=true", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	response := rr.Body.String()
	assert.Contains(t, response, "(abc)")
}

func TestHandlerFeatureIsDisabled(t *testing.T) {

	argoCDCmDisabled := argoCDCm.DeepCopy()
	delete(argoCDCmDisabled.Data, "statusbadge.enabled")

	settingsMgr := settings.NewSettingsManager(context.Background(), fake.NewSimpleClientset(argoCDCmDisabled, &argoCDSecret), "default")
	handler := NewHandler(appclientset.NewSimpleClientset(&testApp), settingsMgr, "default", []string{})
	req, err := http.NewRequest(http.MethodGet, "/api/badge?name=test-app", nil)
	assert.NoError(t, err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, "private, no-store", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))

	response := rr.Body.String()
	assert.Equal(t, toRGBString(Purple), leftRectColorPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, toRGBString(Purple), rightRectColorPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, "Unknown", leftTextPattern.FindStringSubmatch(response)[1])
	assert.Equal(t, "Unknown", rightTextPattern.FindStringSubmatch(response)[1])
}
