package collector

import (
	"strings"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/peimanja/artifactory_exporter/artifactory"
	"github.com/prometheus/client_golang/prometheus"
)

func (e *Exporter) exportSystem(license artifactory.LicenseInfo, ch chan<- prometheus.Metric) error {
	healthy, err := e.client.FetchHealth()
	if err != nil {
		level.Error(e.logger).Log("msg", "Couldn't scrape Artifactory when fetching system/ping", "err", err)
		e.totalAPIErrors.Inc()
		return err
	}
	buildInfo, err := e.client.FetchBuildInfo()
	if err != nil {
		level.Error(e.logger).Log("msg", "Couldn't scrape Artifactory when fetching system/version", "err", err)
		e.totalAPIErrors.Inc()
		return err
	}

	licenseType := strings.ToLower(license.Type)
	for metricName, metric := range systemMetrics {
		switch metricName {
		case "healthy":
			var healthValue float64
			if healthy {
				healthValue = 1
			} else {
				healthValue = 0
			}
			ch <- prometheus.MustNewConstMetric(metric, prometheus.GaugeValue, healthValue)
		case "version":
			ch <- prometheus.MustNewConstMetric(metric, prometheus.GaugeValue, 1, buildInfo.Version, buildInfo.Revision)
		case "license":
			var validThrough float64
			timeNow := float64(time.Now().Unix())
			switch licenseType {
			case "oss":
				validThrough = timeNow
			default:
				if validThroughTime, err := time.Parse("Jan 2, 2006", license.ValidThrough); err != nil {
					level.Warn(e.logger).Log("msg", "Couldn't parse Artifactory license ValidThrough", "err", err)
					validThrough = timeNow
				} else {
					validThrough = float64(validThroughTime.Unix())
				}
			}
			ch <- prometheus.MustNewConstMetric(metric, prometheus.GaugeValue, validThrough-timeNow, licenseType, license.LicensedTo, license.ValidThrough)
		}
	}
	return nil
}
