package main

import (
	"strconv"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// createServiceConfig builds the service manifest for the deployment.
func (r *CheckRunner) createServiceConfig(labels map[string]string) *corev1.Service {
	// Allocate a new service object.
	service := &corev1.Service{}
	log.Infoln("Creating service resource for", r.cfg.CheckNamespace, "namespace.")

	// Build the service ports.
	ports := make([]corev1.ServicePort, 0)
	basicPort := corev1.ServicePort{
		Port: r.cfg.CheckLoadBalancerPort,
		TargetPort: intstr.IntOrString{
			IntVal: r.cfg.CheckContainerPort,
			StrVal: strconv.Itoa(int(r.cfg.CheckContainerPort)),
		},
		Protocol: corev1.ProtocolTCP,
	}
	ports = append(ports, basicPort)

	// Build the service spec.
	serviceSpec := corev1.ServiceSpec{
		Type:     corev1.ServiceTypeClusterIP,
		Ports:    ports,
		Selector: labels,
	}

	// Populate the service metadata.
	service.Spec = serviceSpec
	service.Name = r.cfg.CheckServiceName
	service.Namespace = r.cfg.CheckNamespace

	return service
}
