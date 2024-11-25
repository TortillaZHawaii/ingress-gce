package forwardingrules_test

import (
	"testing"

	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	compute "google.golang.org/api/compute/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/cloud-provider-gcp/providers/gce"
	"k8s.io/ingress-gce/pkg/annotations"
	"k8s.io/ingress-gce/pkg/composite"
	"k8s.io/ingress-gce/pkg/forwardingrules"
	"k8s.io/ingress-gce/pkg/utils"
	"k8s.io/ingress-gce/pkg/utils/namer"
	"k8s.io/klog/v2"
)

const (
	serviceNamespace = "testNs"
	serviceName      = "testSvc"
	bsLink           = "http://www.googleapis.com/projects/test/regions/us-central1/backendServices/bs1"
)

var (
	gceVals gce.TestClusterValues
	l4Namer *namer.L4Namer
)

func getTestMocks() (*gce.Cloud, *namer.L4Namer) {
	gceVals = gce.DefaultTestClusterValues()
	fakeGCE := gce.NewFakeGCECloud(gceVals)

	l4Namer = namer.NewL4Namer("ksuid123", namer.NewNamer(gceVals.ClusterName, "test-fw", klog.TODO()))

	return fakeGCE, l4Namer
}

// func TestEnsureIPv4AddressAlreadyInUse(t *testing.T) {
// 	fakeGCE, namer := getTestMocks()
// 	targetIP := "1.1.1.1"
// 	m := &forwardingrules.ManagerELB{
// 		Namer:    namer,
// 		Provider: forwardingrules.New(fakeGCE, meta.VersionGA, meta.Regional, klog.TODO()),
// 		Logger:   klog.TODO(),
// 		Service:  &api_v1.Service{
// 			ObjectMeta: v1.ObjectMeta{Name: "testService", Namespace: "default", UID: types},
// 		},
// 	}
// }

func TestEnsureIPv4Create(t *testing.T) {
	testCases := []struct {
		desc         string
		svc          *corev1.Service
		namedAddress *compute.Address
		want         *forwardingrules.EnsureELBResult
	}{
		{
			desc: "create tcp",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     8080,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			want: &forwardingrules.EnsureELBResult{
				TCPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "create udp",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     8080,
							Protocol: corev1.ProtocolUDP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			want: &forwardingrules.EnsureELBResult{
				UDPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "create mixed",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     8080,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     8080,
							Protocol: corev1.ProtocolUDP,
						},
						{
							Port:     443,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			want: &forwardingrules.EnsureELBResult{
				TCPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080", "443"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
				UDPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "create range",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     1000,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     2000,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     3000,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     4000,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     5000,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     6000,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     7000,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			want: &forwardingrules.EnsureELBResult{
				TCPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					PortRange:           "1000-7000",
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "create with discrete ports and network tier",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1"), Annotations: map[string]string{annotations.NetworkTierAnnotationKey: string(cloud.NetworkTierStandard)}},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     8080,
							Protocol: corev1.ProtocolUDP,
						},
						{
							Port:     8085,
							Protocol: corev1.ProtocolUDP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			want: &forwardingrules.EnsureELBResult{
				UDPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080", "8085"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "create with assigned IP",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     8080,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type:           "LoadBalancer",
					LoadBalancerIP: "1.1.1.1",
				},
			},
			want: &forwardingrules.EnsureELBResult{
				TCPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					IPAddress:           "1.1.1.1",
					Ports:               []string{"8080"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "1.1.1.1", utils.XLB),
				},
			},
		},
		{
			desc:         "create with named address",
			namedAddress: &compute.Address{Name: "my-addr", Address: "1.2.3.4", AddressType: string(cloud.SchemeExternal)},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1"), Annotations: map[string]string{annotations.StaticL4AddressesAnnotationKey: "my-addr"}},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     8080,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			want: &forwardingrules.EnsureELBResult{
				TCPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					IPAddress:           "1.2.3.4",
					Ports:               []string{"8080"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "1.2.3.4", utils.XLB),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Arrange
			fakeGCE, namer := getTestMocks()
			m := &forwardingrules.ManagerELB{
				Namer:    namer,
				Provider: forwardingrules.New(fakeGCE, meta.VersionGA, meta.Regional, klog.TODO()),
				Recorder: &record.FakeRecorder{},
				Service:  tc.svc,
			}

			if tc.namedAddress != nil {
				fakeGCE.ReserveRegionAddress(tc.namedAddress, fakeGCE.Region())
			}

			// Act
			got, err := m.EnsureIPv4(&forwardingrules.EnsureELBConfig{
				BackendServiceLink: bsLink,
			})
			// Assert
			if err != nil {
				t.Errorf("EnsureIPv4() err=%v", err)
			}
			assertResult(t, got, tc.want)
		})
	}
}

func TestEnsureIPv4Update(t *testing.T) {
	testCases := []struct {
		desc     string
		svc      *corev1.Service
		existing []*composite.ForwardingRule
		want     *forwardingrules.EnsureELBResult
	}{
		{
			desc: "no update, tcp only",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     8080,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			existing: []*composite.ForwardingRule{
				{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
			want: &forwardingrules.EnsureELBResult{
				SyncStatus: utils.ResourceResync,
				TCPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "no update, udp only",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     1080,
							Protocol: corev1.ProtocolUDP,
						},
						{
							Port:     8080,
							Protocol: corev1.ProtocolUDP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			existing: []*composite.ForwardingRule{
				{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"1080", "8080"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
			want: &forwardingrules.EnsureELBResult{
				SyncStatus: utils.ResourceResync,
				UDPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"1080", "8080"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "no update, mixed",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     8080,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     8080,
							Protocol: corev1.ProtocolUDP,
						},
						{
							Port:     443,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			existing: []*composite.ForwardingRule{
				{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080", "443"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
				{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
			want: &forwardingrules.EnsureELBResult{
				SyncStatus: utils.ResourceResync,
				TCPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080", "443"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
				UDPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "update ports",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     8080,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     443,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			existing: []*composite.ForwardingRule{
				{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
			want: &forwardingrules.EnsureELBResult{
				SyncStatus: utils.ResourceUpdate,
				TCPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080", "443"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "change protocol",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     1080,
							Protocol: corev1.ProtocolUDP,
						},
						{
							Port:     2080,
							Protocol: corev1.ProtocolUDP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			existing: []*composite.ForwardingRule{
				{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
			want: &forwardingrules.EnsureELBResult{
				SyncStatus: utils.ResourceUpdate,
				UDPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"1080", "2080"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "mixed to udp only",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     1080,
							Protocol: corev1.ProtocolUDP,
						},
						{
							Port:     2080,
							Protocol: corev1.ProtocolUDP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			existing: []*composite.ForwardingRule{
				{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
				{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"1080", "2080"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
			want: &forwardingrules.EnsureELBResult{
				SyncStatus: utils.ResourceUpdate,
				UDPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"1080", "2080"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
		{
			desc: "mixed to tcp only with port change",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: serviceNamespace, UID: types.UID("1")},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:     1080,
							Protocol: corev1.ProtocolTCP,
						},
						{
							Port:     2080,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Type: "LoadBalancer",
				},
			},
			existing: []*composite.ForwardingRule{
				{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"8080"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
				{
					Name:                "k8s2-udp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"1080", "2080"},
					IPProtocol:          "UDP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
			want: &forwardingrules.EnsureELBResult{
				SyncStatus: utils.ResourceUpdate,
				TCPFwdRule: &composite.ForwardingRule{
					Name:                "k8s2-tcp-axyqjz2d-testNs-testSvc-2ve2wd1r",
					Ports:               []string{"1080", "2080"},
					IPProtocol:          "TCP",
					LoadBalancingScheme: string(cloud.SchemeExternal),
					NetworkTier:         cloud.NetworkTierDefault.ToGCEValue(),
					Version:             meta.VersionGA,
					BackendService:      bsLink,
					Description:         l4ServiceDescription(t, serviceName, serviceNamespace, "", utils.XLB),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// Arrange
			fakeGCE, namer := getTestMocks()
			fw := forwardingrules.New(fakeGCE, meta.VersionGA, meta.Regional, klog.TODO())
			m := &forwardingrules.ManagerELB{
				Namer:    namer,
				Provider: fw,
				Recorder: &record.FakeRecorder{},
				Service:  tc.svc,
			}

			for _, rule := range tc.existing {
				err := fw.Create(rule)
				if err != nil {
					t.Errorf("Failed to set up existing forwarding rule: %v", err)
				}
			}

			// Act
			got, err := m.EnsureIPv4(&forwardingrules.EnsureELBConfig{
				BackendServiceLink: bsLink,
			})
			// Assert
			if err != nil {
				t.Errorf("EnsureIPv4() err=%v", err)
			}
			assertResult(t, got, tc.want)
		})
	}
}

func assertResult(t *testing.T, got, want *forwardingrules.EnsureELBResult) {
	if want.SyncStatus != got.SyncStatus {
		t.Errorf("EnsureIPv4().SyncStatus = %v, want %v", got.SyncStatus, want.SyncStatus)
	}

	if want.TCPFwdRule != nil {
		if got.TCPFwdRule == nil {
			t.Errorf("EnsureIPv4 didn't return expected TCP forwarding rule")
		}
		if diff := cmp.Diff(want.TCPFwdRule, got.TCPFwdRule, cmpopts.IgnoreFields(composite.ForwardingRule{}, "SelfLink", "Region", "Scope")); diff != "" {
			t.Errorf("EnsureIPv4() TCP forwarding rule diff -want +got\n%v\n", diff)
		}
	}

	if want.UDPFwdRule != nil {
		if got.UDPFwdRule == nil {
			t.Errorf("EnsureIPv4 didn't return expected UDP forwarding rule")
		}
		if diff := cmp.Diff(want.UDPFwdRule, got.UDPFwdRule, cmpopts.IgnoreFields(composite.ForwardingRule{}, "SelfLink", "Region", "Scope")); diff != "" {
			t.Errorf("EnsureIPv4() UDP forwarding rule diff -want +got\n%v\n", diff)
		}
	}

	if got.IPManaged != want.IPManaged {
		t.Errorf("EnsureIPv4().IPManaged = %v, want %v", got.IPManaged, want.IPManaged)
	}
}

func l4ServiceDescription(t *testing.T, svcName, svcNamespace, ipToUse string, lbType utils.L4LBType) string {
	description, err := utils.MakeL4LBServiceDescription(utils.ServiceKeyFunc(svcNamespace, svcName), ipToUse,
		meta.VersionGA, false, lbType)
	if err != nil {
		t.Errorf("utils.MakeL4LBServiceDescription() failed, err=%v", err)
	}
	return description
}
