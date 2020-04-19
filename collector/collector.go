package collector

import (
	"strings"

	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "artifactory"
)

var (
	filestoreLabelNames   = []string{"storage_type", "storage_dir"}
	repoLabelNames        = []string{"name", "type", "package_type"}
	replicationLabelNames = []string{"name", "type", "url", "cron_exp"}
)

func newMetric(metricName string, subsystem string, docString string, labelNames []string) *prometheus.Desc {
	return prometheus.NewDesc(prometheus.BuildFQName(namespace, subsystem, metricName), docString, labelNames, nil)
}

type metrics map[string]*prometheus.Desc

var (
	replicationMetrics = metrics{
		"enabled": newMetric("enabled", "replication", "Replication status for an Artifactory repository (1 = enabled).", replicationLabelNames),
	}

	securityMetrics = metrics{
		"users":  newMetric("users", "security", "Number of Artifactory users for each realm.", []string{"realm"}),
		"groups": newMetric("groups", "security", "Number of Artifactory groups", nil),
	}

	storageMetrics = metrics{
		"artifacts":      newMetric("artifacts", "storage", "Total artifacts count stored in Artifactory.", nil),
		"artifactsSize":  newMetric("artifacts_size_bytes", "storage", "Total artifacts Size stored in Artifactory in bytes.", nil),
		"binaries":       newMetric("binaries", "storage", "Total binaries count stored in Artifactory.", nil),
		"binariesSize":   newMetric("binaries_size_bytes", "storage", "Total binaries Size stored in Artifactory in bytes.", nil),
		"filestore":      newMetric("filestore_bytes", "storage", "Total available space in the file store in bytes.", filestoreLabelNames),
		"filestoreUsed":  newMetric("filestore_used_bytes", "storage", "Used space in the file store in bytes.", filestoreLabelNames),
		"filestoreFree":  newMetric("filestore_free_bytes", "storage", "Free space in the file store in bytes.", filestoreLabelNames),
		"repoUsed":       newMetric("repo_used_bytes", "storage", "Used space by an Artifactory repository in bytes.", repoLabelNames),
		"repoFolders":    newMetric("repo_folders", "storage", "Number of folders in an Artifactory repository.", repoLabelNames),
		"repoFiles":      newMetric("repo_files", "storage", "Number files in an Artifactory repository.", repoLabelNames),
		"repoItems":      newMetric("repo_items", "storage", "Number Items in an Artifactory repository.", repoLabelNames),
		"repoPercentage": newMetric("repo_percentage", "storage", "Percentage of space used by an Artifactory repository.", repoLabelNames),
	}

	systemMetrics = metrics{
		"healthy": newMetric("healthy", "system", "Is Artifactory working properly (1 = healthy).", nil),
		"version": newMetric("version", "system", "Version and revision of Artifactory as labels.", []string{"version", "revision"}),
		"license": newMetric("license", "system", "License type and expiry as labels, seconds to expiration as value", []string{"type", "licensed_to", "expires"}),
	}

	artifactsMetrics = metrics{
		"created1m":     newMetric("created_1m", "artifacts", "Number of artifacts created in the repository in the last 1 minute.", repoLabelNames),
		"created5m":     newMetric("created_5m", "artifacts", "Number of artifacts created in the repository in the last 5 minutes.", repoLabelNames),
		"created15m":    newMetric("created_15m", "artifacts", "Number of artifacts created in the repository in the last 15 minutes.", repoLabelNames),
		"downloaded1m":  newMetric("downloaded_1m", "artifacts", "Number of artifacts downloaded from the repository in the last 1 minute.", repoLabelNames),
		"downloaded5m":  newMetric("downloaded_5m", "artifacts", "Number of artifacts downloaded from the repository in the last 5 minutes.", repoLabelNames),
		"downloaded15m": newMetric("downloaded_15m", "artifacts", "Number of artifacts downloaded from the repository in the last 15 minutes.", repoLabelNames),
	}

	artifactoryUp = newMetric("up", "", "Was the last scrape of Artifactory successful.", nil)
)

// Describe describes all the metrics ever exported by the Artifactory exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range replicationMetrics {
		ch <- m
	}
	for _, m := range securityMetrics {
		ch <- m
	}
	for _, m := range storageMetrics {
		ch <- m
	}
	for _, m := range systemMetrics {
		ch <- m
	}
	for _, m := range artifactsMetrics {
		ch <- m
	}
	ch <- artifactoryUp
	ch <- e.totalScrapes.Desc()
	ch <- e.jsonParseFailures.Desc()
}

// Collect fetches the stats from  Artifactory and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()

	up := e.scrape(ch)

	ch <- prometheus.MustNewConstMetric(artifactoryUp, prometheus.GaugeValue, up)
	ch <- e.totalScrapes
	ch <- e.jsonParseFailures
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) (up float64) {
	e.totalScrapes.Inc()

	// Collect License info
	var licenseType string
	license, err := e.fetchLicense()
	if err != nil {
		level.Error(e.logger).Log("msg", "Couldn't scrape Artifactory when fetching system/license", "err", err)
		return 0
	}
	licenseType = strings.ToLower(license.Type)
	// Some API endpoints are not available in OSS
	if licenseType != "oss" {
		for metricName, metric := range securityMetrics {
			switch metricName {
			case "users":
				err := e.exportUsersCount(metricName, metric, ch)
				if err != nil {
					return 0
				}
			case "groups":
				err := e.exportGroups(metricName, metric, ch)
				if err != nil {
					return 0
				}
			}
		}
		err = e.exportReplications(ch)
		if err != nil {
			return 0
		}
	}

	// Collect and export system metrics
	err = e.exportSystem(license, ch)
	if err != nil {
		return 0
	}

	// Fetch Storage Info stats and register them
	storageInfo, err := e.fetchStorageInfo()
	if err != nil {
		level.Error(e.logger).Log("msg", "Couldn't scrape Artifactory when fetching storageinfo", "err", err)
		return 0
	}
	e.exportStorage(storageInfo, ch)

	// Extract repo summaries from storageInfo and register them
	repoSummaryList, err := e.extractRepo(storageInfo)
	if err != nil {
		return 0
	}
	e.exportRepo(repoSummaryList, ch)

	// Get Downloaded and Created items for all repo in the last 1 and 5 minutes and add it to repoSummaryList
	repoSummaryList, err = e.getTotalArtifacts(repoSummaryList)
	if err != nil {
		return 0
	}
	e.exportArtifacts(repoSummaryList, ch)

	return 1
}
