package graph

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

// Resources is a raw, read-only snapshot of collected cluster objects that the
// builder turns into graph nodes and edges.
type Resources struct {
	Deployments  []appsv1.Deployment
	ReplicaSets  []appsv1.ReplicaSet
	StatefulSets []appsv1.StatefulSet
	DaemonSets   []appsv1.DaemonSet
	Jobs         []batchv1.Job
	CronJobs     []batchv1.CronJob

	Pods      []corev1.Pod
	Services  []corev1.Service
	Ingresses []networkingv1.Ingress

	ConfigMaps          []corev1.ConfigMap
	Secrets             []corev1.Secret
	PVCs                []corev1.PersistentVolumeClaim
	PVs                 []corev1.PersistentVolume
	Nodes               []corev1.Node
	ServiceAccounts     []corev1.ServiceAccount
	NetworkPolicies     []networkingv1.NetworkPolicy
	Namespaces          []corev1.Namespace
	Roles               []rbacv1.Role
	ClusterRoles        []rbacv1.ClusterRole
	RoleBindings        []rbacv1.RoleBinding
	ClusterRoleBindings []rbacv1.ClusterRoleBinding
}

// builder accumulates nodes and a name index while constructing the graph so
// edges can resolve references (name → UID) without scanning the whole graph.
type builder struct {
	g     *Graph
	index map[nameKey]types.UID
	seen  map[edgeKey]bool
}

type nameKey struct {
	kind, namespace, name string
}

type edgeKey struct {
	from, to types.UID
	rel      Relationship
}

// Build constructs a graph from a resource snapshot. Ownership edges come from
// metadata.ownerReferences; the rest are derived from selectors, references,
// mounts, scheduling, and storage bindings. Names are never hardcoded.
func Build(r Resources) *Graph {
	b := &builder{
		g:     New(),
		index: make(map[nameKey]types.UID),
		seen:  make(map[edgeKey]bool),
	}

	b.addNodes(r)
	b.addOwnerEdges(r)
	b.addServiceEdges(r)
	b.addIngressEdges(r)
	b.addPodEdges(r)
	b.addStorageEdges(r)
	b.addNetworkPolicyEdges(r)
	b.addRBACEdges(r)

	return b.g
}

func (b *builder) addNode(kind, namespace, name string, uid types.UID, raw any) {
	b.g.AddNode(Node{UID: uid, Kind: kind, Namespace: namespace, Name: name, Raw: raw})
	b.index[nameKey{kind, namespace, name}] = uid
}

func (b *builder) edge(from, to types.UID, rel Relationship) {
	if from == "" || to == "" {
		return
	}
	k := edgeKey{from, to, rel}
	if b.seen[k] {
		return
	}
	b.seen[k] = true
	b.g.AddEdge(from, to, rel)
}

func (b *builder) lookup(kind, namespace, name string) types.UID {
	return b.index[nameKey{kind, namespace, name}]
}

func (b *builder) addNodes(r Resources) {
	for i := range r.Deployments {
		o := &r.Deployments[i]
		b.addNode("Deployment", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.ReplicaSets {
		o := &r.ReplicaSets[i]
		b.addNode("ReplicaSet", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.StatefulSets {
		o := &r.StatefulSets[i]
		b.addNode("StatefulSet", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.DaemonSets {
		o := &r.DaemonSets[i]
		b.addNode("DaemonSet", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.Jobs {
		o := &r.Jobs[i]
		b.addNode("Job", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.CronJobs {
		o := &r.CronJobs[i]
		b.addNode("CronJob", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.Pods {
		o := &r.Pods[i]
		b.addNode("Pod", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.Services {
		o := &r.Services[i]
		b.addNode("Service", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.Ingresses {
		o := &r.Ingresses[i]
		b.addNode("Ingress", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.ConfigMaps {
		o := &r.ConfigMaps[i]
		b.addNode("ConfigMap", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.Secrets {
		o := &r.Secrets[i]
		b.addNode("Secret", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.PVCs {
		o := &r.PVCs[i]
		b.addNode("PersistentVolumeClaim", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.PVs {
		o := &r.PVs[i]
		b.addNode("PersistentVolume", "", o.Name, o.UID, o)
	}
	for i := range r.Nodes {
		o := &r.Nodes[i]
		b.addNode("Node", "", o.Name, o.UID, o)
	}
	for i := range r.ServiceAccounts {
		o := &r.ServiceAccounts[i]
		b.addNode("ServiceAccount", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.NetworkPolicies {
		o := &r.NetworkPolicies[i]
		b.addNode("NetworkPolicy", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.Namespaces {
		o := &r.Namespaces[i]
		b.addNode("Namespace", "", o.Name, o.UID, o)
	}
	for i := range r.Roles {
		o := &r.Roles[i]
		b.addNode("Role", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.ClusterRoles {
		o := &r.ClusterRoles[i]
		b.addNode("ClusterRole", "", o.Name, o.UID, o)
	}
	for i := range r.RoleBindings {
		o := &r.RoleBindings[i]
		b.addNode("RoleBinding", o.Namespace, o.Name, o.UID, o)
	}
	for i := range r.ClusterRoleBindings {
		o := &r.ClusterRoleBindings[i]
		b.addNode("ClusterRoleBinding", "", o.Name, o.UID, o)
	}
}

// addOwnerEdges links owners to the objects they own via ownerReferences
// (Deployment→ReplicaSet→Pod, StatefulSet/DaemonSet→Pod, CronJob→Job→Pod).
func (b *builder) addOwnerEdges(r Resources) {
	link := func(owned metav1.Object, owners []metav1.OwnerReference) {
		for _, o := range owners {
			owner, ok := b.g.Node(o.UID)
			if !ok {
				continue
			}
			// Skip Node→Pod ownership (kubeadm mirror static pods reference the
			// Node): it fans every workload's dependency tree out to unrelated
			// control-plane pods. "What runs on a node" is the reverse of the
			// Pod→Node scheduling edge instead.
			if owner.Kind == "Node" {
				continue
			}
			b.edge(o.UID, owned.GetUID(), RelOwns)
		}
	}
	for i := range r.ReplicaSets {
		link(&r.ReplicaSets[i], r.ReplicaSets[i].OwnerReferences)
	}
	for i := range r.Jobs {
		link(&r.Jobs[i], r.Jobs[i].OwnerReferences)
	}
	for i := range r.Pods {
		link(&r.Pods[i], r.Pods[i].OwnerReferences)
	}
}

// addServiceEdges links a Service to Pods whose labels match its selector.
func (b *builder) addServiceEdges(r Resources) {
	for i := range r.Services {
		svc := &r.Services[i]
		if len(svc.Spec.Selector) == 0 {
			continue
		}
		sel := labels.SelectorFromSet(svc.Spec.Selector)
		for j := range r.Pods {
			p := &r.Pods[j]
			if p.Namespace == svc.Namespace && sel.Matches(labels.Set(p.Labels)) {
				b.edge(svc.UID, p.UID, RelSelects)
			}
		}
	}
}

// addIngressEdges links an Ingress to the Services it routes to.
func (b *builder) addIngressEdges(r Resources) {
	for i := range r.Ingresses {
		ing := &r.Ingresses[i]
		add := func(svcName string) {
			if svcName == "" {
				return
			}
			b.edge(ing.UID, b.lookup("Service", ing.Namespace, svcName), RelRoutes)
		}
		if db := ing.Spec.DefaultBackend; db != nil && db.Service != nil {
			add(db.Service.Name)
		}
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil {
					add(path.Backend.Service.Name)
				}
			}
		}
	}
}

// addPodEdges links Pods to the ConfigMaps, Secrets, PVCs, ServiceAccount, and
// Node they reference.
func (b *builder) addPodEdges(r Resources) {
	for i := range r.Pods {
		p := &r.Pods[i]
		ns := p.Namespace

		cfg := func(name string) { b.edge(p.UID, b.lookup("ConfigMap", ns, name), RelMounts) }
		sec := func(name string) { b.edge(p.UID, b.lookup("Secret", ns, name), RelMounts) }
		pvc := func(name string) { b.edge(p.UID, b.lookup("PersistentVolumeClaim", ns, name), RelMounts) }

		for _, v := range p.Spec.Volumes {
			switch {
			case v.ConfigMap != nil:
				cfg(v.ConfigMap.Name)
			case v.Secret != nil:
				sec(v.Secret.SecretName)
			case v.PersistentVolumeClaim != nil:
				pvc(v.PersistentVolumeClaim.ClaimName)
			case v.Projected != nil:
				for _, s := range v.Projected.Sources {
					if s.ConfigMap != nil {
						cfg(s.ConfigMap.Name)
					}
					if s.Secret != nil {
						sec(s.Secret.Name)
					}
				}
			}
		}

		containers := append([]corev1.Container{}, p.Spec.InitContainers...)
		containers = append(containers, p.Spec.Containers...)
		for _, c := range containers {
			for _, ef := range c.EnvFrom {
				if ef.ConfigMapRef != nil {
					cfg(ef.ConfigMapRef.Name)
				}
				if ef.SecretRef != nil {
					sec(ef.SecretRef.Name)
				}
			}
			for _, e := range c.Env {
				if e.ValueFrom == nil {
					continue
				}
				if e.ValueFrom.ConfigMapKeyRef != nil {
					cfg(e.ValueFrom.ConfigMapKeyRef.Name)
				}
				if e.ValueFrom.SecretKeyRef != nil {
					sec(e.ValueFrom.SecretKeyRef.Name)
				}
			}
		}

		for _, ips := range p.Spec.ImagePullSecrets {
			sec(ips.Name)
		}

		if sa := p.Spec.ServiceAccountName; sa != "" {
			b.edge(p.UID, b.lookup("ServiceAccount", ns, sa), RelUses)
		}
		if node := p.Spec.NodeName; node != "" {
			b.edge(p.UID, b.lookup("Node", "", node), RelScheduled)
		}
	}
}

// addStorageEdges binds PersistentVolumeClaims to their PersistentVolumes.
func (b *builder) addStorageEdges(r Resources) {
	for i := range r.PVCs {
		pvc := &r.PVCs[i]
		if pvc.Spec.VolumeName != "" {
			b.edge(pvc.UID, b.lookup("PersistentVolume", "", pvc.Spec.VolumeName), RelBinds)
		}
	}
}

// addNetworkPolicyEdges creates NetworkPolicy→Pod edges for every Pod whose
// labels match a policy's spec.podSelector within the same namespace.
func (b *builder) addNetworkPolicyEdges(r Resources) {
	for i := range r.NetworkPolicies {
		np := &r.NetworkPolicies[i]
		sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
		if err != nil {
			continue
		}
		for j := range r.Pods {
			p := &r.Pods[j]
			if p.Namespace == np.Namespace && sel.Matches(labels.Set(p.Labels)) {
				b.edge(np.UID, p.UID, RelPolicySelects)
			}
		}
	}
}

// addRBACEdges creates RoleBinding→ServiceAccount (grants) and
// RoleBinding→Role/ClusterRole (role ref) edges.
func (b *builder) addRBACEdges(r Resources) {
	for i := range r.RoleBindings {
		rb := &r.RoleBindings[i]
		// Edge to the referenced role or cluster role.
		switch rb.RoleRef.Kind {
		case "Role":
			roleUID := b.lookup("Role", rb.Namespace, rb.RoleRef.Name)
			b.edge(rb.UID, roleUID, RelRoleRef)
		case "ClusterRole":
			crUID := b.lookup("ClusterRole", "", rb.RoleRef.Name)
			b.edge(rb.UID, crUID, RelRoleRef)
		}
		// Edge to each ServiceAccount subject.
		for _, subj := range rb.Subjects {
			if subj.Kind != "ServiceAccount" {
				continue
			}
			ns := subj.Namespace
			if ns == "" {
				ns = rb.Namespace
			}
			saUID := b.lookup("ServiceAccount", ns, subj.Name)
			b.edge(rb.UID, saUID, RelGrants)
		}
	}
	for i := range r.ClusterRoleBindings {
		crb := &r.ClusterRoleBindings[i]
		crUID := b.lookup("ClusterRole", "", crb.RoleRef.Name)
		b.edge(crb.UID, crUID, RelRoleRef)
		for _, subj := range crb.Subjects {
			if subj.Kind != "ServiceAccount" {
				continue
			}
			saUID := b.lookup("ServiceAccount", subj.Namespace, subj.Name)
			b.edge(crb.UID, saUID, RelGrants)
		}
	}
}
