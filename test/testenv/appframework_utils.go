package testenv

import (
	"context"
	"fmt"
	"strings"

	splcommon "github.com/splunk/splunk-operator/pkg/splunk/common"

	enterpriseApi "github.com/splunk/splunk-operator/api/v3"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

//AppInfo Installation info on Apps
var AppInfo = map[string]map[string]string{
	"Splunk_SA_CIM":                     {"V1": "4.18.1", "V2": "4.19.0", "filename": "splunk-common-information-model-cim.tgz"},
	"TA-LDAP":                           {"V1": "4.0.0", "V2": "4.0.0", "filename": "add-on-for-ldap.tgz"},
	"DA-ESS-ContentUpdate":              {"V1": "3.20.0", "V2": "3.21.0", "filename": "splunk-es-content-update.tgz"},
	"Splunk_TA_paloalto":                {"V1": "6.6.0", "V2": "7.0.0", "filename": "palo-alto-networks-add-on-for-splunk.tgz"},
	"TA-MS-AAD":                         {"V1": "3.0.0", "V2": "3.1.1", "filename": "microsoft-azure-add-on-for-splunk.tgz"},
	"Splunk_TA_nix":                     {"V1": "8.3.0", "V2": "8.3.1", "filename": "splunk-add-on-for-unix-and-linux.tgz"},
	"splunk_app_microsoft_exchange":     {"V1": "4.0.1", "V2": "4.0.2", "filename": "splunk-app-for-microsoft-exchange.tgz"},
	"splunk_app_aws":                    {"V1": "6.0.1", "V2": "6.0.2", "filename": "splunk-app-for-aws.tgz"},
	"Splunk_ML_Toolkit":                 {"V1": "5.2.0", "V2": "5.2.1", "filename": "splunk-machine-learning-toolkit.tgz"},
	"Splunk_TA_microsoft-cloudservices": {"V1": "4.1.2", "V2": "4.1.3", "filename": "splunk-add-on-for-microsoft-cloud-services.tgz"},
	"splunk_app_stream":                 {"V1": "7.3.0", "V2": "7.4.0", "filename": "splunk-app-for-stream.tgz"},
	"Splunk_TA_stream_wire_data":        {"V1": "7.3.0", "V2": "7.4.0", "filename": "splunk-add-on-for-stream-wire-data.tgz"},
	"Splunk_TA_stream":                  {"V1": "7.3.0", "V2": "7.4.0", "filename": "splunk-add-on-for-stream-forwarders.tgz"},
	"splunk_app_db_connect":             {"V1": "3.5.0", "V2": "3.5.1", "filename": "splunk-db-connect.tgz"},
	"Splunk_Security_Essentials":        {"V1": "3.3.2", "V2": "3.3.3", "filename": "splunk-security-essentials.tgz"},
	"SplunkEnterpriseSecuritySuite":     {"V1": "6.4.0", "V2": "6.4.1", "filename": "splunk-enterprise-security.spl"},
}

//BasicApps Apps that require no restart to be installed
var BasicApps = []string{"Splunk_SA_CIM", "DA-ESS-ContentUpdate", "Splunk_TA_paloalto", "TA-MS-AAD"}

//RestartNeededApps Apps that require restart to be installed
var RestartNeededApps = []string{"Splunk_TA_nix", "splunk_app_microsoft_exchange", "splunk_app_aws", "Splunk_ML_Toolkit", "Splunk_TA_microsoft-cloudservices", "splunk_app_stream", "Splunk_TA_stream", "Splunk_TA_stream_wire_data", "splunk_app_db_connect", "Splunk_Security_Essentials"}

//NewAppsAddedBetweenPolls Apps to be installed as poll after
var NewAppsAddedBetweenPolls = []string{"TA-LDAP"}

// AppLocationV1 Location of apps on S3 for V1 Apps
var AppLocationV1 = "appframework/v1apps/"

// AppLocationV2 Location of apps on S3 for V2 Apps
var AppLocationV2 = "appframework/v2apps/"

// GenerateAppSourceSpec return AppSourceSpec struct with given values
func GenerateAppSourceSpec(appSourceName string, appSourceLocation string, appSourceDefaultSpec enterpriseApi.AppSourceDefaultSpec) enterpriseApi.AppSourceSpec {
	return enterpriseApi.AppSourceSpec{
		Name:                 appSourceName,
		Location:             appSourceLocation,
		AppSourceDefaultSpec: appSourceDefaultSpec,
	}
}

// GetPodAppStatus Get the app install status and version number
func GetPodAppStatus(ctx context.Context, deployment *Deployment, podName string, ns string, appname string, clusterWideInstall bool) (string, string, error) {
	// For clusterwide install do not check for versions on deployer and cluster-manager as the apps arent installed there
	if clusterWideInstall && (strings.Contains(podName, splcommon.TestClusterManagerDashed) || strings.Contains(podName, splcommon.TestDeployerDashed)) {
		logf.Log.Info("Pod skipped as install is Cluter-wide", "PodName", podName)
		return "", "", nil
	}
	output, err := GetPodAppInstallStatus(ctx, deployment, podName, ns, appname)
	if err != nil {
		return "", "", err
	}
	status := strings.Fields(output)[2]
	version, err := GetPodInstalledAppVersion(deployment, podName, ns, appname, clusterWideInstall)
	return status, version, err

}

// GetPodInstalledAppVersion Get the version of the app installed on pod
func GetPodInstalledAppVersion(deployment *Deployment, podName string, ns string, appname string, clusterWideInstall bool) (string, error) {
	path := "etc/apps"
	//For cluster-wide install the apps are extracted to different locations
	if clusterWideInstall {
		if strings.Contains(podName, "-indexer-") {
			path = splcommon.PeerAppsLoc
		} else if strings.Contains(podName, splcommon.ClusterManager) {
			path = splcommon.ManagerAppsLoc
		} else if strings.Contains(podName, splcommon.TestDeployerDashed) {
			path = splcommon.SHClusterAppsLoc
		}
	}
	filePath := fmt.Sprintf("/opt/splunk/%s/%s/default/app.conf", path, appname)
	logf.Log.Info("Check app version", "App", appname, "Conf file", filePath)

	confline, err := GetConfLineFromPod(podName, filePath, ns, "version", "launcher", true)
	if err != nil {
		logf.Log.Error(err, "Failed to get version from pod", "Pod Name", podName)
		return "", err
	}
	version := strings.TrimSpace(strings.Split(confline, "=")[1])
	return version, err

}

// GetPodAppInstallStatus Get the app install status
func GetPodAppInstallStatus(ctx context.Context, deployment *Deployment, podName string, ns string, appname string) (string, error) {
	stdin := fmt.Sprintf("/opt/splunk/bin/splunk display app '%s' -auth admin:$(cat /mnt/splunk-secrets/password)", appname)
	command := []string{"/bin/sh"}
	stdout, stderr, err := deployment.PodExecCommand(ctx, podName, command, stdin, false)
	if err != nil {
		logf.Log.Error(err, "Failed to execute command on pod", "pod", podName, "command", command, "stdin", stdin)
		return "", err
	}
	logf.Log.Info("Command executed", "on pod", podName, "command", command, "stdin", stdin, "stdout", stdout, "stderr", stderr)

	return strings.TrimSuffix(stdout, "\n"), nil
}

// GetPodAppbtoolStatus Get the app btool status
func GetPodAppbtoolStatus(ctx context.Context, deployment *Deployment, podName string, ns string, appname string) (string, error) {
	stdin := fmt.Sprintf("/opt/splunk/bin/splunk btool %s --app=%s --debug", appname, appname)
	command := []string{"/bin/sh"}
	stdout, stderr, err := deployment.PodExecCommand(ctx, podName, command, stdin, false)
	if err != nil {
		logf.Log.Error(err, "Failed to execute command on pod", "pod", podName, "command", command, "stdin", stdin)
		return "", err
	}
	logf.Log.Info("Command executed", "on pod", podName, "command", command, "stdin", stdin, "stdout", stdout, "stderr", stderr)

	if len(stdout) > 0 {
		if strings.Contains(strings.Split(stdout, "\n")[0], "App is disabled") {
			return "DISABLED", nil
		}
		return "ENABLED", nil
	}
	return "", err
}

// GetAppFileList Get the Versioned App file list for  app Names
func GetAppFileList(appList []string) []string {
	appFileList := make([]string, 0, len(appList))
	for _, app := range appList {
		appFileList = append(appFileList, AppInfo[app]["filename"])
	}
	return appFileList
}

// GenerateAppFrameworkSpec Generate Appframework spec
func GenerateAppFrameworkSpec(testenvInstance *TestCaseEnv, volumeName string, scope string, appSourceName string, s3TestDir string, pollInterval int) enterpriseApi.AppFrameworkSpec {

	// Create App framework volume
	volumeSpec := []enterpriseApi.VolumeSpec{GenerateIndexVolumeSpec(volumeName, GetS3Endpoint(), testenvInstance.GetIndexSecretName(), "aws", "s3")}

	// AppSourceDefaultSpec: Remote Storage volume name and Scope of App deployment
	appSourceDefaultSpec := enterpriseApi.AppSourceDefaultSpec{
		VolName: volumeName,
		Scope:   scope,
	}

	// appSourceSpec: App source name, location and volume name and scope from appSourceDefaultSpec
	appSourceSpec := []enterpriseApi.AppSourceSpec{GenerateAppSourceSpec(appSourceName, s3TestDir, appSourceDefaultSpec)}

	// appFrameworkSpec: AppSource settings, Poll Interval, volumes, appSources on volumes
	appFrameworkSpec := enterpriseApi.AppFrameworkSpec{
		Defaults:             appSourceDefaultSpec,
		AppsRepoPollInterval: int64(pollInterval),
		VolList:              volumeSpec,
		AppSources:           appSourceSpec,
	}

	return appFrameworkSpec
}
