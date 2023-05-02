package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	listenAddress   = kingpin.Flag("web.listen-address", "Address to listen on for web interface and telemetry.").Envar("WEB_LISTEN_ADDR").Default(":9531").String()
	metricsPath     = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Envar("WEB_TELEMETRY_PATH").Default("/metrics").String()
	artiScrapeURI   = kingpin.Flag("artifactory.scrape-uri", "URI on which to scrape JFrog Artifactory.").Envar("ARTI_SCRAPE_URI").Default("http://localhost:8081/artifactory").String()
	artiSSLVerify   = kingpin.Flag("artifactory.ssl-verify", "Flag that enables SSL certificate verification for the scrape URI").Envar("ARTI_SSL_VERIFY").Default("false").Bool()
	artiTimeout     = kingpin.Flag("artifactory.timeout", "Timeout for trying to get stats from JFrog Artifactory.").Envar("ARTI_TIMEOUT").Default("5s").Duration()
	optionalMetrics = kingpin.Flag("optional-metric", "optional metric to be enabled. Pass multiple times to enable multiple optional metrics.").PlaceHolder("metric-name").Strings()
)

var optionalMetricsList = []string{"artifacts", "replication_status", "federation_status"}

// Credentials represents Username and Password or API Key for
// Artifactory Authentication
type Credentials struct {
	AuthMethod  string
	Username    string `required:"false" envconfig:"ARTI_USERNAME"`
	Password    string `required:"false" envconfig:"ARTI_PASSWORD"`
	AccessToken string `required:"false" envconfig:"ARTI_ACCESS_TOKEN"`
}

type OptionalMetrics struct {
	Artifacts         bool
	ReplicationStatus bool
	FederationStatus  bool
}

// Config represents all configuration options for running the Exporter.
type Config struct {
	ListenAddress   string
	MetricsPath     string
	ArtiScrapeURI   string
	Credentials     *Credentials
	ArtiSSLVerify   bool
	ArtiTimeout     time.Duration
	OptionalMetrics OptionalMetrics
	Logger          log.Logger
}

// NewConfig Creates new Artifactory exporter Config
func NewConfig() (*Config, error) {

	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.HelpFlag.Short('h')
	kingpin.Version(version.Info() + " " + version.BuildContext())
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	var credentials Credentials
	err := envconfig.Process("", &credentials)
	if err != nil {
		return nil, err
	}
	if credentials.Username != "" && credentials.Password != "" && credentials.AccessToken == "" {
		credentials.AuthMethod = "userPass"
	} else if credentials.Username == "" && credentials.Password == "" && credentials.AccessToken != "" {
		credentials.AuthMethod = "accessToken"
	} else {
		return nil, fmt.Errorf("`ARTI_USERNAME` and `ARTI_PASSWORD` or `ARTI_ACCESS_TOKEN` environment variable hast to be set")
	}

	_, err = url.Parse(*artiScrapeURI)
	if err != nil {
		return nil, err
	}

	optMetrics := OptionalMetrics{}
	for _, metric := range *optionalMetrics {
		switch metric {
		case "artifacts":
			optMetrics.Artifacts = true
		case "replication_status":
			optMetrics.ReplicationStatus = true
		case "federation_status":
			optMetrics.FederationStatus = true
		default:
			return nil, fmt.Errorf("unknown optional metric: %s. Valid optional metrics are: %s", metric, optionalMetricsList)
		}
	}

	return &Config{
		ListenAddress:   *listenAddress,
		MetricsPath:     *metricsPath,
		ArtiScrapeURI:   *artiScrapeURI,
		Credentials:     &credentials,
		ArtiSSLVerify:   *artiSSLVerify,
		ArtiTimeout:     *artiTimeout,
		OptionalMetrics: optMetrics,
		Logger:          logger,
	}, nil

}
