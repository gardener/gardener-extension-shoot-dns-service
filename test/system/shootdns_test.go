// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package system_test

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/extensions"
	"github.com/gardener/gardener/test/framework"
	. "github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-extension-shoot-dns-service/test/resources/templates"
)

var testCfg *ShootCPTestConfig

// ShootCPTestConfig holds configuration for shoot tests using the control plane
type ShootCPTestConfig struct {
	ShootKubeconfig  string
	SeedKubeconfig   string
	ShootName        string
	ProjectNamespace string
}

func init() {
	testCfg = RegisterShootCPTestFlags()
}

// RegisterShootCPTestFlags registers flags for ShootCPTestConfig
func RegisterShootCPTestFlags() *ShootCPTestConfig {
	newCfg := &ShootCPTestConfig{}

	flag.StringVar(&newCfg.ShootKubeconfig, "shoot-kubecfg", "", "the path with the shoot kubeconfig.")
	flag.StringVar(&newCfg.SeedKubeconfig, "seed-kubecfg", "", "the path with the seed kubeconfig.")
	flag.StringVar(&newCfg.ShootName, "shoot-name", "", "the shoot name")
	flag.StringVar(&newCfg.ProjectNamespace, "project-namespace", "", "the project namespace of the shoot")

	return newCfg
}

type shootDNSFramework struct {
	*framework.CommonFramework
	config ShootCPTestConfig

	seedClient  kubernetes.Interface
	shootClient kubernetes.Interface
	cluster     *extensions.Cluster
}

func newShootDNSFramework(_ *framework.CommonConfig) *shootDNSFramework {
	return &shootDNSFramework{
		CommonFramework: framework.NewCommonFramework(&framework.CommonConfig{
			ResourceDir: "../resources",
		}),
		config: *testCfg,
	}
}

func (f *shootDNSFramework) technicalShootId() string {
	middle := strings.TrimPrefix(f.config.ProjectNamespace, "garden-")
	return fmt.Sprintf("shoot--%s--%s", middle, f.config.ShootName)
}

func (f *shootDNSFramework) prepareClientsAndCluster() {
	var err error
	f.seedClient, err = kubernetes.NewClientFromFile("", f.config.SeedKubeconfig,
		kubernetes.WithClientOptions(client.Options{Scheme: kubernetes.SeedScheme}),
		kubernetes.WithDisabledCachedClient(),
	)
	framework.ExpectNoError(err)
	f.shootClient, err = kubernetes.NewClientFromFile("", f.config.ShootKubeconfig,
		kubernetes.WithClientOptions(client.Options{Scheme: kubernetes.ShootScheme}),
		kubernetes.WithDisabledCachedClient(),
	)
	framework.ExpectNoError(err)

	f.cluster, err = controller.GetCluster(context.TODO(), f.seedClient.Client(), f.technicalShootId())
	framework.ExpectNoError(err)
	if !f.cluster.Shoot.Spec.Addons.NginxIngress.Enabled {
		Fail("The test requires .spec.addons.nginxIngress.enabled to be true")
	}
	if f.cluster.Shoot.Spec.DNS == nil || f.cluster.Shoot.Spec.DNS.Domain == nil {
		Fail("The test requires .spec.dns.domain to be set")
	}
}

func (f *shootDNSFramework) createNamespace(ctx context.Context, namespace string) *v1.Namespace {
	f.Logger.Info("Using namespace", "namespaceName", namespace)
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	err := f.shootClient.Client().Create(ctx, ns)
	framework.ExpectNoError(err)

	return ns
}

func (f *shootDNSFramework) deleteNamespaceAndWait(ctx context.Context, ns *v1.Namespace) {
	f.Logger.Info("Deleting namespace", "namespaceName", ns.Name)
	err := f.shootClient.Client().Delete(ctx, ns)
	framework.ExpectNoError(err)
	err = f.WaitUntilNamespaceIsDeleted(ctx, f.shootClient, ns.Name)
	framework.ExpectNoError(err)
	f.Logger.Info("Deleted namespace", "namespaceName", ns.Name)
}

func (f *shootDNSFramework) createEchoheaders(ctx context.Context, svcLB, delete bool,
	timeoutDNS time.Duration, timeoutHttp time.Duration) {
	suffix := "ingress"
	if svcLB {
		suffix = "service-lb"
	}
	namespace := fmt.Sprintf("shootdns-test-echoserver-%s", suffix)
	ns := f.createNamespace(ctx, namespace)

	values := map[string]interface{}{
		"EchoName":                fmt.Sprintf("echo-%s", suffix),
		"Namespace":               namespace,
		"ShootDnsName":            *f.cluster.Shoot.Spec.DNS.Domain,
		"ServiceTypeLoadBalancer": svcLB,
	}
	err := f.RenderAndDeployTemplate(ctx, f.shootClient, templates.EchoserverApp, values)
	framework.ExpectNoError(err)

	domainName := fmt.Sprintf("%s.%s", values["EchoName"], values["ShootDnsName"])
	err = awaitDNSRecord(domainName, timeoutDNS)
	framework.ExpectNoError(err)
	err = runHttpRequest(domainName, timeoutHttp)
	framework.ExpectNoError(err)

	if delete {
		f.deleteNamespaceAndWait(ctx, ns)
	} else {
		f.Logger.Info("No cleanup of namespace", "namespaceName", ns.Name)
	}
}

var _ = Describe("ShootDNS test", func() {

	f := newShootDNSFramework(&framework.CommonConfig{
		ResourceDir: "../resources",
	})

	BeforeEach(f.prepareClientsAndCluster)

	framework.CIt("Create and delete echoheaders service with type LoadBalancer", func(ctx context.Context) {
		f.createEchoheaders(ctx, true, true, 360*time.Second, 420*time.Second)
	}, 840*time.Second)

	framework.CIt("Create echoheaders ingress", func(ctx context.Context) {
		// cleanup during shoot deletion to test proper cleanup
		f.createEchoheaders(ctx, false, false, 180*time.Second, 420*time.Second)
	}, 660*time.Second)

	framework.CIt("Create custom DNS entry", func(ctx context.Context) {
		namespace := "shootdns-test-custom-dnsentry"
		ns := f.createNamespace(ctx, namespace)

		domainName := "custom." + *f.cluster.Shoot.Spec.DNS.Domain
		values := map[string]interface{}{
			"Namespace": namespace,
			"DNSName":   domainName,
		}
		err := f.RenderAndDeployTemplate(ctx, f.shootClient, templates.CustomDNSEntry, values)
		framework.ExpectNoError(err)

		err = awaitDNSRecord(domainName, 120*time.Second)
		framework.ExpectNoError(err)

		f.deleteNamespaceAndWait(ctx, ns)
	}, 90*time.Second)
})

func await(f func() error, sleep, timeout time.Duration) error {
	end := time.Now().Add(timeout)

	var err error
	for time.Now().Before(end) {
		time.Sleep(sleep)
		err = f()
		if err == nil {
			return nil
		}
	}
	return err
}

func awaitDNSRecord(domainName string, timeout time.Duration) error {
	// first make a DNS lookup to avoid long waiting time because of negative DNS caching
	err := await(func() error {
		_, err := lookupHost(domainName, "8.8.8.8")
		return err
	}, 3*time.Second, timeout)
	if err != nil {
		return fmt.Errorf("lookup host %s failed: %w", domainName, err)
	}
	return nil
}

func runHttpRequest(domainName string, timeout time.Duration) error {
	err := await(func() error {
		url := fmt.Sprintf("http://%s", domainName)
		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("get request failed for %s: %w", url, err)
		}
		err = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("resp.Body.Close failed: %w", err)
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("unexpected status code: %d %s", resp.StatusCode, resp.Status)
		}
		return nil
	}, 3*time.Second, timeout)
	return err
}

func lookupHost(host, dnsServer string) ([]string, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: time.Millisecond * time.Duration(10000),
			}
			return d.DialContext(ctx, network, fmt.Sprintf("%s:53", dnsServer))
		},
	}
	return r.LookupHost(context.Background(), host)
}
