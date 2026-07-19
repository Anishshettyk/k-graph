// Package tui is the interactive terminal UI for KGraph, built on Bubble Tea.
// It presents a filterable resource list, a live colored dependency graph, and
// a details/health panel, plus a fuzzy context switcher. It is presentational
// only — all analysis comes from the graph/query/health packages.
package tui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	corev1 "k8s.io/api/core/v1"

	"github.com/anishetty/kgraph/internal/diagnosis"
	"github.com/anishetty/kgraph/internal/graph"
	"github.com/anishetty/kgraph/internal/health"
	"github.com/anishetty/kgraph/internal/query"
	"github.com/anishetty/kgraph/internal/renderer"
	"k8s.io/apimachinery/pkg/types"
)

// Loader rebuilds the graph and pod usage for a given kubeconfig context.
// Usage is best-effort: an empty map is fine when metrics are unavailable.
type Loader func(contextName string) (*graph.Graph, map[string]Usage, error)

// Diagnoser reads evidence for one selected resource. It runs asynchronously
// so Kubernetes event and log reads never block TUI navigation.
type Diagnoser func(context.Context, string, graph.Node) (diagnosis.Report, error)

// Usage holds live CPU/memory usage and limits for a pod (millicores / bytes).
type Usage struct {
	CPUMilli      int64
	MemBytes      int64
	CPULimitMilli int64
	MemLimitBytes int64
}

type mode int

const (
	modeDeps mode = iota
	modeImpact
)

type screen int

const (
	screenGraph screen = iota
	screenContext
	screenNamespace
	screenProblems
	screenOrphans
	screenSettings
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("57")).Padding(0, 1)
	depsBadge   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("42")).Padding(0, 1)
	impactBadge = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("214")).Padding(0, 1)
	subtle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	faint       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("57")).Padding(0, 1)
	panelTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	badStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	keyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
)

// gauge renders a horizontal utilization bar like "████░░░░ 62%", colored by
// severity. pct is 0-100.
func gauge(pct, width int) string {
	if width < 6 {
		width = 6
	}
	if pct < 0 {
		pct = 0
	}
	filled := pct * width / 100
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	st := okStyle
	switch {
	case pct >= 90:
		st = badStyle
	case pct >= 75:
		st = warnStyle
	}
	return st.Render(bar) + fmt.Sprintf(" %3d%%", pct)
}

// Model is the Bubble Tea model for the explorer.
type Model struct {
	g              *graph.Graph
	load           Loader
	diagnose       Diagnoser
	usage          map[string]Usage
	contexts       []string
	currentContext string

	resources     list.Model
	contextList   list.Model
	namespaceList list.Model
	problems      list.Model
	orphans       list.Model
	viewport      viewport.Model
	problemView   viewport.Model
	orphanView    viewport.Model
	spinner       spinner.Model
	command       textinput.Model

	mode       mode
	screen     screen
	selected   *graph.Node
	loading    bool
	commanding bool
	kindFilter string
	namespace  string
	problem    *graph.Node
	orphanSel  *graph.Node
	reports    map[types.UID]diagnosis.Report
	diagErrs   map[types.UID]error
	diagnosing types.UID

	width, height int
	ready         bool
	err           error
}

// loadedMsg is delivered when an async context reload completes.
type loadedMsg struct {
	g     *graph.Graph
	usage map[string]Usage
	name  string
	err   error
}

// diagnosedMsg is delivered when evidence collection for one resource ends.
type diagnosedMsg struct {
	uid    types.UID
	report diagnosis.Report
	err    error
}

// New constructs the explorer model.
func New(g *graph.Graph, usage map[string]Usage, currentContext string, contexts []string, load Loader, diagnose Diagnoser) Model {
	if usage == nil {
		usage = map[string]Usage{}
	}

	resList := list.New(nil, resourceDelegate{usage: usage}, 0, 0)
	resList.Title = "Resources"
	resList.SetShowHelp(false)
	resList.SetShowStatusBar(false)
	resList.SetStatusBarItemName("resource", "resources")

	ctxList := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	ctxList.Title = "Switch context"
	ctxList.SetShowHelp(false)

	nsList := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	nsList.Title = "Select namespace"
	nsList.SetShowHelp(false)

	problemList := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	problemList.Title = "Problems"
	problemList.SetShowHelp(false)
	problemList.SetShowStatusBar(false)

	orphanList := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	orphanList.Title = "Orphans"
	orphanList.SetShowHelp(false)
	orphanList.SetShowStatusBar(false)
	orphanList.SetStatusBarItemName("orphan", "orphans")

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Prompt = ":"
	ti.Placeholder = "pods, deploy, svc, ns, ctx, all…"
	ti.CharLimit = 40

	m := Model{
		g:              g,
		load:           load,
		diagnose:       diagnose,
		usage:          usage,
		contexts:       contexts,
		currentContext: currentContext,
		resources:      resList,
		contextList:    ctxList,
		namespaceList:  nsList,
		problems:       problemList,
		orphans:        orphanList,
		viewport:       viewport.New(0, 0),
		problemView:    viewport.New(0, 0),
		orphanView:     viewport.New(0, 0),
		spinner:        sp,
		command:        ti,
		mode:           modeDeps,
		screen:         screenGraph,
		reports:        make(map[types.UID]diagnosis.Report),
		diagErrs:       make(map[types.UID]error),
	}
	m.resources.SetItems(nodeItems(g, "", ""))
	m.contextList.SetItems(ctxItems(contexts))
	m.namespaceList.SetItems(namespaceItems(g))
	m.problems.SetItems(problemItems(g, ""))
	m.orphans.SetItems(orphanItems(g, ""))
	m.syncSelection()
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return m.diagnoseCmd() }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.layout()
		m.refreshTree()
		return m, nil

	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.g = msg.g
		m.usage = msg.usage
		if m.usage == nil {
			m.usage = map[string]Usage{}
		}
		m.currentContext = msg.name
		m.reports = make(map[types.UID]diagnosis.Report)
		m.diagErrs = make(map[types.UID]error)
		m.diagnosing = ""
		m.resources.SetDelegate(resourceDelegate{usage: m.usage})
		m.resources.SetItems(nodeItems(msg.g, m.kindFilter, m.namespace))
		m.namespaceList.SetItems(namespaceItems(msg.g))
		m.problems.SetItems(problemItems(msg.g, m.namespace))
		m.orphans.SetItems(orphanItems(msg.g, m.namespace))
		m.problems.SetItems(problemItems(msg.g, m.namespace))
		m.resources.Select(0)
		m.syncSelection()
		m.refreshTree()
		return m, m.diagnoseCmd()

	case diagnosedMsg:
		if m.diagnosing == msg.uid {
			m.diagnosing = ""
		}
		m.reports[msg.uid] = msg.report
		if msg.err != nil {
			m.diagErrs[msg.uid] = msg.err
		} else {
			delete(m.diagErrs, msg.uid)
		}
		if m.screen == screenProblems {
			m.refreshProblemView()
		}
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	switch m.screen {
	case screenContext:
		m.contextList, cmd = m.contextList.Update(msg)
	case screenNamespace:
		m.namespaceList, cmd = m.namespaceList.Update(msg)
	case screenProblems:
		m.problems, cmd = m.problems.Update(msg)
	case screenOrphans:
		m.orphans, cmd = m.orphans.Update(msg)
	case screenSettings:
		// Settings screen is non-interactive; only esc closes it.
		m.resources, cmd = m.resources.Update(msg)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Command prompt (k9s-style ":") owns all keys while active.
	if m.commanding {
		switch msg.String() {
		case "esc":
			m.commanding = false
			m.command.Blur()
			return m, nil
		case "enter":
			raw := m.command.Value()
			m.commanding = false
			m.command.Blur()
			return m.runCommand(raw)
		}
		var cmd tea.Cmd
		m.command, cmd = m.command.Update(msg)
		return m, cmd
	}

	if m.screen == screenContext {
		switch msg.String() {
		case "esc":
			m.screen = screenGraph
			return m, nil
		case "enter":
			if it, ok := m.contextList.SelectedItem().(ctxItem); ok {
				m.screen = screenGraph
				m.loading = true
				m.err = nil
				return m, tea.Batch(m.spinner.Tick, m.loadCmd(string(it)))
			}
		}
		var cmd tea.Cmd
		m.contextList, cmd = m.contextList.Update(msg)
		return m, cmd
	}
	if m.screen == screenNamespace {
		switch msg.String() {
		case "esc":
			m.screen = screenGraph
			return m, nil
		case "enter":
			if it, ok := m.namespaceList.SelectedItem().(namespaceItem); ok {
				m.namespace = it.namespace
				m.screen = screenGraph
				return m, m.applyResources()
			}
		}
		var cmd tea.Cmd
		m.namespaceList, cmd = m.namespaceList.Update(msg)
		return m, cmd
	}
	if m.screen == screenProblems {
		if msg.String() == "esc" {
			m.screen = screenGraph
			return m, nil
		}
		var cmd tea.Cmd
		m.problems, cmd = m.problems.Update(msg)
		changed := m.syncProblemSelection()
		m.refreshProblemView()
		if changed {
			return m, tea.Batch(cmd, m.diagnoseCmdFor(m.problem))
		}
		return m, cmd
	}
	if m.screen == screenOrphans {
		if msg.String() == "esc" {
			m.screen = screenGraph
			return m, nil
		}
		var cmd tea.Cmd
		m.orphans, cmd = m.orphans.Update(msg)
		m.syncOrphanSelection()
		m.refreshOrphanView()
		return m, cmd
	}
	if m.screen == screenSettings {
		m.screen = screenGraph
		return m, nil
	}

	if m.resources.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.resources, cmd = m.resources.Update(msg)
		changed := m.syncSelection()
		m.refreshTree()
		if changed {
			return m, tea.Batch(cmd, m.diagnoseCmd())
		}
		return m, cmd
	}

	switch msg.String() {
	case ":":
		m.commanding = true
		m.err = nil
		m.command.SetValue("")
		m.command.Focus()
		return m, textinput.Blink
	case "q", "esc":
		return m, tea.Quit
	case "tab":
		if m.mode == modeDeps {
			m.mode = modeImpact
		} else {
			m.mode = modeDeps
		}
		m.refreshTree()
		return m, nil
	case "c":
		m.screen = screenContext
		return m, nil
	case "p":
		return m, m.openProblems()
	case "o":
		return m, m.openOrphans()
	case "?":
		m.screen = screenSettings
		return m, nil
	case "pgup", "pgdown", " ", "b", "f":
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.resources, cmd = m.resources.Update(msg)
	changed := m.syncSelection()
	m.refreshTree()
	if changed {
		return m, tea.Batch(cmd, m.diagnoseCmd())
	}
	return m, cmd
}

// kindCommands maps k9s-style command aliases to canonical kinds.
var kindCommands = map[string]string{
	"po": "Pod", "pod": "Pod", "pods": "Pod",
	"deploy": "Deployment", "deployment": "Deployment", "deployments": "Deployment",
	"rs": "ReplicaSet", "replicaset": "ReplicaSet", "replicasets": "ReplicaSet",
	"sts": "StatefulSet", "statefulset": "StatefulSet", "statefulsets": "StatefulSet",
	"ds": "DaemonSet", "daemonset": "DaemonSet", "daemonsets": "DaemonSet",
	"job": "Job", "jobs": "Job",
	"cj": "CronJob", "cronjob": "CronJob", "cronjobs": "CronJob",
	"svc": "Service", "service": "Service", "services": "Service",
	"ing": "Ingress", "ingress": "Ingress", "ingresses": "Ingress",
	"cm": "ConfigMap", "configmap": "ConfigMap", "configmaps": "ConfigMap",
	"secret": "Secret", "secrets": "Secret",
	"pvc": "PersistentVolumeClaim", "pv": "PersistentVolume",
	"no": "Node", "node": "Node", "nodes": "Node",
	"sa": "ServiceAccount", "serviceaccount": "ServiceAccount", "serviceaccounts": "ServiceAccount",
}

// runCommand interprets a ":" command: a kind alias filters the resource list,
// ns opens the namespace picker, ctx opens the context switcher, all clears the
// filters, and q quits.
func (m Model) runCommand(raw string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(strings.TrimSpace(strings.TrimPrefix(raw, ":")))
	if len(parts) == 0 {
		return m, nil
	}
	cmd := strings.ToLower(parts[0])
	switch cmd {
	case "q", "quit", "exit":
		return m, tea.Quit
	case "all", "*":
		m.kindFilter = ""
		m.namespace = ""
		return m, m.applyResources()
	case "ctx", "context", "contexts":
		m.screen = screenContext
		return m, nil
	case "ns", "namespace":
		if len(parts) != 1 {
			m.err = fmt.Errorf("use :ns to choose a namespace")
			return m, nil
		}
		m.namespaceList.SetItems(namespaceItems(m.g))
		m.namespaceList.Select(namespaceItemIndex(m.namespaceList.Items(), m.namespace))
		m.screen = screenNamespace
		return m, nil
	case "problems", "problem", "issues":
		if len(parts) != 1 {
			m.err = fmt.Errorf("use :problems to view current-scope problems")
			return m, nil
		}
		return m, m.openProblems()
	case "orphan", "orphans":
		if len(parts) != 1 {
			m.err = fmt.Errorf("use :orphan to view unused resources")
			return m, nil
		}
		return m, m.openOrphans()
	}
	if len(parts) != 1 {
		m.err = fmt.Errorf("unknown command %q", strings.TrimSpace(raw))
		return m, nil
	}
	if kind, ok := kindCommands[cmd]; ok {
		m.kindFilter = kind
		return m, m.applyResources()
	}
	m.err = fmt.Errorf("unknown command %q — try a kind, :problems, :orphan, :ns, ctx, all, or q", cmd)
	return m, nil
}

// applyResources repopulates the list from the current graph and kind filter.
func (m *Model) applyResources() tea.Cmd {
	m.resources.SetItems(nodeItems(m.g, m.kindFilter, m.namespace))
	m.resources.Select(0)
	m.syncSelection()
	m.refreshTree()
	return m.diagnoseCmd()
}

func (m *Model) openOrphans() tea.Cmd {
	m.orphans.SetItems(orphanItems(m.g, m.namespace))
	m.orphans.Select(0)
	m.syncOrphanSelection()
	m.refreshOrphanView()
	m.screen = screenOrphans
	return nil
}

func (m *Model) syncOrphanSelection() {
	if it, ok := m.orphans.SelectedItem().(orphanItem); ok {
		n := it.node
		m.orphanSel = &n
	} else {
		m.orphanSel = nil
	}
}

func (m *Model) refreshOrphanView() {
	if m.orphanSel == nil {
		scope := "all namespaces"
		if m.namespace != "" {
			scope = "namespace " + m.namespace
		}
		m.orphanView.SetContent(subtle.Render("No orphaned resources found in " + scope + "."))
		return
	}
	var content strings.Builder
	node := *m.orphanSel
	fmt.Fprintf(&content, "%s%s/%s\n", panelTitle.Render("Orphaned resource "), node.Namespace, node.Name)
	// Show which resources this node provides to (outgoing edges) — why it
	// looks like something might still depend on it.
	var depTree bytes.Buffer
	renderer.Tree(&depTree, query.DependencyTree(m.g, node), renderer.Options{Color: true, Annotate: badge})
	if depTree.Len() > 0 {
		content.WriteString("\n")
		content.WriteString(panelTitle.Render("Outgoing edges (potential remaining users)"))
		content.WriteString("\n")
		content.WriteString(depTree.String())
	}
	// Show what currently uses this resource (incoming edges — should be empty
	// for a true orphan, but shows context if it is partially referenced).
	var impTree bytes.Buffer
	renderer.Tree(&impTree, query.ImpactTree(m.g, node), renderer.Options{Color: true, Annotate: badge})
	if impTree.Len() > 0 {
		content.WriteString("\n")
		content.WriteString(panelTitle.Render("Incoming edges (referenced by)"))
		content.WriteString("\n")
		content.WriteString(impTree.String())
	} else {
		content.WriteString("\n")
		content.WriteString(okStyle.Render("● No incoming edges — safe to review for deletion"))
		content.WriteString("\n")
	}
	m.orphanView.SetContent(content.String())
	m.orphanView.GotoTop()
}

func (m *Model) openProblems() tea.Cmd {
	m.problems.SetItems(problemItems(m.g, m.namespace))
	m.problems.Select(0)
	m.syncProblemSelection()
	m.refreshProblemView()
	m.screen = screenProblems
	return m.diagnoseCmdFor(m.problem)
}

func (m Model) loadCmd(name string) tea.Cmd {
	return func() tea.Msg {
		g, usage, err := m.load(name)
		return loadedMsg{g: g, usage: usage, name: name, err: err}
	}
}

func (m *Model) syncSelection() bool {
	previous := types.UID("")
	if m.selected != nil {
		previous = m.selected.UID
	}
	if it, ok := m.resources.SelectedItem().(nodeItem); ok {
		n := it.node
		m.selected = &n
		return previous != n.UID
	}
	m.selected = nil
	return previous != ""
}

func (m *Model) syncProblemSelection() bool {
	previous := types.UID("")
	if m.problem != nil {
		previous = m.problem.UID
	}
	if item, ok := m.problems.SelectedItem().(problemItem); ok {
		node := item.node
		m.problem = &node
		return previous != node.UID
	}
	m.problem = nil
	return previous != ""
}

func (m *Model) diagnoseCmd() tea.Cmd {
	return m.diagnoseCmdFor(m.diagnosisTarget())
}

func (m *Model) diagnoseCmdFor(target *graph.Node) tea.Cmd {
	if m.diagnose == nil || target == nil {
		return nil
	}
	if _, ok := m.reports[target.UID]; ok || m.diagnosing == target.UID {
		return nil
	}
	m.diagnosing = target.UID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		report, err := m.diagnose(ctx, m.currentContext, *target)
		return diagnosedMsg{uid: target.UID, report: report, err: err}
	}
}

// diagnosisTarget returns the selected Pod or the first unhealthy Pod reached
// from a higher-level resource through graph relationships.
func (m Model) diagnosisTarget() *graph.Node {
	if m.selected == nil {
		return nil
	}
	if m.selected.Kind == "Pod" {
		n := *m.selected
		return &n
	}

	seen := map[types.UID]bool{m.selected.UID: true}
	queue := append([]types.UID(nil), m.g.Out(m.selected.UID)...)
	for len(queue) > 0 {
		uid := queue[0]
		queue = queue[1:]
		if seen[uid] {
			continue
		}
		seen[uid] = true
		n, ok := m.g.Node(uid)
		if !ok {
			continue
		}
		if n.Kind == "Pod" {
			if healthy, _ := health.Pod(n); !healthy {
				return &n
			}
			continue
		}
		queue = append(queue, m.g.Out(uid)...)
	}
	return nil
}

func (m *Model) refreshTree() {
	if m.selected == nil {
		m.viewport.SetContent(subtle.Render("Select a resource on the left."))
		return
	}
	var t *query.TreeNode
	if m.mode == modeDeps {
		t = query.DependencyTree(m.g, *m.selected)
	} else {
		t = query.ImpactTree(m.g, *m.selected)
	}

	var buf bytes.Buffer
	renderer.Tree(&buf, t, renderer.Options{Color: true, Annotate: badge})
	content := buf.String()
	if len(t.Children) == 0 {
		if m.mode == modeDeps {
			content += subtle.Render("\n  (nothing to depend on)")
		} else {
			content += subtle.Render("\n  (nothing depends on this)")
		}
	}
	m.viewport.SetContent(content)
	m.viewport.GotoTop()
}

func (m *Model) refreshProblemView() {
	if m.problem == nil {
		scope := "all namespaces"
		if m.namespace != "" {
			scope = "namespace " + m.namespace
		}
		m.problemView.SetContent(subtle.Render("No problems found in " + scope + "."))
		return
	}

	var content strings.Builder
	node := *m.problem
	fmt.Fprintf(&content, "%s%s/%s\n", panelTitle.Render("Problem "), node.Namespace, node.Name)

	// State line: Pod health comes from the status-phase check; other kinds get
	// the reason from the problem list (e.g. Pending / no matching Pods).
	switch node.Kind {
	case "Pod":
		if _, reason := health.Pod(node); reason != "" {
			fmt.Fprintf(&content, "%s%s\n", keyStyle.Render("state "), badStyle.Render("✖ "+reason))
		}
	default:
		for _, item := range m.problems.Items() {
			if pi, ok := item.(problemItem); ok && pi.node.UID == node.UID {
				fmt.Fprintf(&content, "%s%s\n", keyStyle.Render("state "), badStyle.Render("✖ "+pi.reason))
				break
			}
		}
	}

	m.writeDiagnosis(&content, node)

	// For Pods show the reverse-dep tree (what's affected). For PVCs and
	// Services show both directions: what they depend on and what uses them.
	content.WriteString("\n")
	if node.Kind == "Pod" {
		content.WriteString(panelTitle.Render("Affected resources"))
		content.WriteString("\n")
		var tree bytes.Buffer
		renderer.Tree(&tree, query.ImpactTree(m.g, node), renderer.Options{Color: true, Annotate: badge})
		content.WriteString(tree.String())
	} else {
		content.WriteString(panelTitle.Render("Dependencies (what it needs)"))
		content.WriteString("\n")
		var deps bytes.Buffer
		renderer.Tree(&deps, query.DependencyTree(m.g, node), renderer.Options{Color: true, Annotate: badge})
		content.WriteString(deps.String())
		content.WriteString("\n")
		content.WriteString(panelTitle.Render("Affected resources (what uses it)"))
		content.WriteString("\n")
		var imp bytes.Buffer
		renderer.Tree(&imp, query.ImpactTree(m.g, node), renderer.Options{Color: true, Annotate: badge})
		content.WriteString(imp.String())
	}

	m.problemView.SetContent(content.String())
	m.problemView.GotoTop()
}

func (m *Model) layout() {
	headerH := 4 // title + hotkeys + jump/prompt + spare
	footerH := 1
	bodyH := m.height - headerH - footerH
	if bodyH < 4 {
		bodyH = 4
	}

	listW := clamp(m.width/4, 24, 40)
	showDetails := m.width >= 96
	detailsOuterW := 0
	if showDetails {
		// The details panel adds two border and two padding columns around its
		// declared content width.
		detailsOuterW = clamp(m.width/4, 26, 42) + 4
	}
	graphOuter := m.width - listW - detailsOuterW - 2
	if graphOuter < 16 {
		graphOuter = 16
	}

	m.resources.SetSize(listW, bodyH)
	m.contextList.SetSize(m.width-2, bodyH)
	m.namespaceList.SetSize(m.width-2, bodyH)
	m.problems.SetSize(listW, bodyH)
	m.orphans.SetSize(listW, bodyH)

	m.viewport.Width = graphOuter - 4
	vpH := bodyH - 3
	if vpH < 3 {
		vpH = 3
	}
	m.viewport.Height = vpH
	problemOuter := m.width - listW - 1
	if problemOuter < 16 {
		problemOuter = 16
	}
	m.problemView.Width = problemOuter - 4
	m.problemView.Height = vpH
	m.orphanView.Width = problemOuter - 4
	m.orphanView.Height = vpH
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	switch m.screen {
	case screenContext:
		return m.contextView()
	case screenNamespace:
		return m.namespaceView()
	case screenProblems:
		return m.problemsView()
	case screenOrphans:
		return m.orphansView()
	case screenSettings:
		return m.settingsView()
	default:
		return m.graphView()
	}
}

func (m Model) graphView() string {
	graphTitle := "Graph"
	if m.selected != nil {
		graphTitle = renderer.Icon(m.selected.Kind) + " " + m.selected.Kind + " " + m.selected.Name
	}
	graphPanel := lipgloss.JoinVertical(lipgloss.Left,
		panelTitle.Render(graphTitle),
		panelStyle.Render(m.viewport.View()),
	)

	columns := []string{m.resources.View(), " ", graphPanel}
	if m.width >= 96 {
		columns = append(columns, " ", m.detailsPanel())
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, columns...)

	help := subtle.Render("↑/↓ select · tab deps⇄impact · p problems · o orphans · pgup/pgdn scroll · / filter · c context · ? settings · q quit")
	if m.err != nil {
		help = errStyle.Render("error: "+truncate(m.err.Error(), m.width-8)) + "\n" + help
	}

	return lipgloss.JoinVertical(lipgloss.Left, m.headerView(), body, help)
}

// jumpHints teaches the k9s-style ":" commands for fast resource switching.
const jumpHints = ":pods  :deploy  :sts  :ds  :job  :svc  :ing  :cm  :secret  :pvc  :node  :sa  :ns  :problems  :orphan  :all"

// headerView renders the k9s-style top bar: cluster/context info + mode + a
// hotkey menu, and either the active command prompt or the quick-jump hints.
func (m Model) headerView() string {
	modeText := "deps"
	badgeStyle := depsBadge
	if m.mode == modeImpact {
		modeText = "impact"
		badgeStyle = impactBadge
	}

	ctxText := "⎈ " + m.currentContext
	if m.loading {
		ctxText = m.spinner.View() + " " + m.currentContext + "…"
	}
	view := "all"
	if m.kindFilter != "" {
		view = m.kindFilter
	}
	if m.namespace != "" {
		view += " · ns: " + m.namespace
	}

	line1 := lipgloss.JoinHorizontal(lipgloss.Center,
		titleStyle.Render("KGraph"), " ",
		subtle.Render(ctxText), "  ",
		badgeStyle.Render(modeText), "  ",
		keyStyle.Render("view: ")+view, "  ",
		m.summaryLine(),
	)

	keys := faint.Render(":cmd · /filter · ↑↓ nav · tab deps⇄impact · c contexts · q quit")

	var line3 string
	if m.commanding {
		line3 = m.command.View()
	} else {
		line3 = faint.Render("jump: ") + subtle.Render(jumpHints)
	}

	return lipgloss.JoinVertical(lipgloss.Left, line1, keys, line3)
}

// summaryLine reports cluster-wide pod counts so problems are visible at a
// glance: total pods, unhealthy, and pods at/over their limit.
func (m Model) summaryLine() string {
	var pods, unhealthy, overLimit int
	for _, n := range m.g.Nodes() {
		if n.Kind != "Pod" {
			continue
		}
		pods++
		if ok, _ := health.Pod(n); !ok {
			unhealthy++
		}
		if u, ok := m.usage[n.Namespace+"/"+n.Name]; ok {
			if overThreshold(u.CPUMilli, u.CPULimitMilli, 90) || overThreshold(u.MemBytes, u.MemLimitBytes, 90) {
				overLimit++
			}
		}
	}

	parts := []string{okStyle.Render(fmt.Sprintf("%d pods", pods))}
	if unhealthy > 0 {
		parts = append(parts, badStyle.Render(fmt.Sprintf("✖ %d unhealthy", unhealthy)))
	} else {
		parts = append(parts, subtle.Render("✓ all ready"))
	}
	if overLimit > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("▲ %d near limit", overLimit)))
	}
	return strings.Join(parts, subtle.Render(" · "))
}

func overThreshold(used, limit int64, pct int) bool {
	return limit > 0 && used*100/limit >= int64(pct)
}

func (m Model) detailsPanel() string {
	var b strings.Builder
	if m.selected == nil {
		b.WriteString(subtle.Render("No selection"))
	} else {
		n := *m.selected
		b.WriteString(panelTitle.Render("Details"))
		b.WriteString("\n\n")
		fmt.Fprintf(&b, "%s%s\n", keyStyle.Render("kind "), n.Kind)
		if n.Namespace != "" {
			fmt.Fprintf(&b, "%s%s\n", keyStyle.Render("ns   "), n.Namespace)
		}
		fmt.Fprintf(&b, "%s%s\n", keyStyle.Render("name "), n.Name)

		if n.Kind == "Pod" {
			if ok, reason := health.Pod(n); ok {
				fmt.Fprintf(&b, "%s%s\n", keyStyle.Render("state "), okStyle.Render("● Ready"))
			} else {
				fmt.Fprintf(&b, "%s%s\n", keyStyle.Render("state "), badStyle.Render("✖ "+reason))
			}
			if pod, ok := n.Raw.(*corev1.Pod); ok && pod != nil {
				if pod.Spec.NodeName != "" {
					fmt.Fprintf(&b, "%s%s\n", keyStyle.Render("node "), pod.Spec.NodeName)
				}
				for _, c := range pod.Spec.Containers {
					fmt.Fprintf(&b, "%s\n", faint.Render("  • "+c.Name+": "+c.Image))
				}
			}
			if u, ok := m.usage[n.Namespace+"/"+n.Name]; ok {
				b.WriteString("\n")
				b.WriteString(panelTitle.Render("Live usage"))
				b.WriteString("\n")
				b.WriteString(metricLine("cpu", u.CPUMilli, u.CPULimitMilli, fmtMilli))
				b.WriteString(metricLine("mem", u.MemBytes, u.MemLimitBytes, fmtBytes))
			}
		}

		if target := m.diagnosisTarget(); target != nil {
			m.writeDiagnosis(&b, *target)
		}

		deps := len(query.Dependencies(m.g, n.UID))
		imp := len(query.Impact(m.g, n.UID))
		b.WriteString("\n")
		fmt.Fprintf(&b, "%s%d\n", keyStyle.Render("depends on   "), deps)
		fmt.Fprintf(&b, "%s%d\n", keyStyle.Render("depended on  "), imp)
	}

	w := clamp(m.width/4, 26, 42)
	// Height applies to the panel content; reserve two rows for its border so
	// the bordered details pane fits the same body height as the other panes.
	bodyH := m.height - 5
	h := bodyH - 2
	if h < 2 {
		h = 2
	}
	return panelStyle.Width(w).Height(h).Render(b.String())
}

func (m Model) writeDiagnosis(b *strings.Builder, target graph.Node) {
	b.WriteString("\n")
	b.WriteString(panelTitle.Render("Why it is failing"))
	b.WriteString("\n")
	if m.diagnosing == target.UID {
		b.WriteString(faint.Render("Analyzing status, events, and logs…"))
		b.WriteString("\n")
		return
	}

	report, known := m.reports[target.UID]
	if known && !report.Empty() {
		if target.Name != m.selected.Name || target.Kind != m.selected.Kind {
			fmt.Fprintf(b, "%s%s\n", keyStyle.Render("pod  "), target.Name)
		}
		b.WriteString(badStyle.Render("✖ " + report.Summary))
		b.WriteString("\n")
		for _, finding := range report.Findings {
			fmt.Fprintf(b, "%s%s\n", warnStyle.Render(finding.Title+":"), truncate(finding.Detail, 110))
		}
		for _, event := range report.Events {
			fmt.Fprintf(b, "%s%s\n", keyStyle.Render("event "+event.Reason+":"), truncate(event.Message, 110))
		}
		for _, log := range report.Logs {
			fmt.Fprintf(b, "%s%s\n", keyStyle.Render("log "+log.Container+":"), truncate(lastLines(log.Excerpt, 3), 160))
		}
		return
	}
	if err, ok := m.diagErrs[target.UID]; ok {
		b.WriteString(faint.Render("Evidence unavailable: " + truncate(err.Error(), 90)))
		b.WriteString("\n")
		return
	}
	b.WriteString(okStyle.Render("● No failure evidence found"))
	b.WriteString("\n")
}

func lastLines(text string, count int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) <= count {
		return strings.Join(lines, " | ")
	}
	return strings.Join(lines[len(lines)-count:], " | ")
}

func (m Model) contextView() string {
	header := titleStyle.Render("KGraph — switch context")
	help := subtle.Render("↑/↓ select · enter switch · / filter · esc cancel")
	return lipgloss.JoinVertical(lipgloss.Left, header, m.contextList.View(), help)
}

func (m Model) namespaceView() string {
	header := titleStyle.Render("KGraph — select namespace")
	help := subtle.Render("↑/↓ select · enter apply · / filter · esc cancel")
	return lipgloss.JoinVertical(lipgloss.Left, header, m.namespaceList.View(), help)
}

func (m Model) problemsView() string {
	graphTitle := "Problem analysis"
	if m.problem != nil {
		graphTitle = renderer.Icon(m.problem.Kind) + " " + m.problem.Namespace + "/" + m.problem.Name
	}
	title := "KGraph problems · unhealthy Pods / PVCs / Services"
	if m.namespace != "" {
		title += " · ns: " + m.namespace
	}
	analysis := lipgloss.JoinVertical(lipgloss.Left,
		panelTitle.Render(graphTitle),
		panelStyle.Render(m.problemView.View()),
	)
	help := subtle.Render("↑/↓ select problem · pgup/pgdn scroll analysis · esc return")
	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		lipgloss.JoinHorizontal(lipgloss.Top, m.problems.View(), " ", analysis),
		help,
	)
}

// settingsView renders a fullscreen help + current-settings panel.
func (m Model) settingsView() string {
	titleSt := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("57")).Padding(0, 1)
	header := titleSt.Render("KGraph — Settings & Help")

	row := func(key, desc string) string {
		return fmt.Sprintf("  %-22s %s",
			keyStyle.Render(key),
			subtle.Render(desc))
	}
	sep := faint.Render("  " + strings.Repeat("─", 54))

	lines := []string{
		header, "",
		panelTitle.Render("  Current configuration"),
		row("context:", m.currentContext),
	}
	ns := m.namespace
	if ns == "" {
		ns = "all namespaces"
	}
	lines = append(lines,
		row("namespace:", ns),
		"",
		sep,
		"",
		panelTitle.Render("  Navigation"),
		row("↑ / ↓", "select resource in list"),
		row("tab", "toggle deps ⇄ impact graph"),
		row("pgup / pgdn", "scroll graph pane"),
		row("/", "fuzzy filter resources"),
		"",
		sep,
		"",
		panelTitle.Render("  Screens"),
		row("p  or  :problems", "unhealthy Pods / PVCs / Services"),
		row("o  or  :orphan", "unused ConfigMaps / Secrets / PVCs"),
		row("c", "switch kubeconfig context"),
		row(":ns", "pick namespace scope"),
		row("?", "this settings screen (any key to close)"),
		"",
		sep,
		"",
		panelTitle.Render("  Command prompt (press :)"),
		row(":pods :deploy :sts", "filter by kind"),
		row(":svc :ing :cm", "filter by kind"),
		row(":secret :pvc :node :sa", "filter by kind"),
		row(":ns", "open namespace picker"),
		row(":problems  :orphan", "jump to problem / orphan view"),
		row(":all", "clear all filters"),
		row(":q", "quit"),
		"",
		sep,
		"",
		faint.Render("  any key to close"),
	)

	return strings.Join(lines, "\n")
}

func (m Model) orphansView() string {
	graphTitle := "Orphan analysis"
	if m.orphanSel != nil {
		graphTitle = renderer.Icon(m.orphanSel.Kind) + " " + m.orphanSel.Namespace + "/" + m.orphanSel.Name
	}
	title := "KGraph orphans · unused ConfigMaps / Secrets / PVCs / Services / ServiceAccounts"
	if m.namespace != "" {
		title += " · ns: " + m.namespace
	}
	analysis := lipgloss.JoinVertical(lipgloss.Left,
		panelTitle.Render(graphTitle),
		panelStyle.Render(m.orphanView.View()),
	)
	help := subtle.Render("↑/↓ select orphan · pgup/pgdn scroll analysis · esc return")
	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		lipgloss.JoinHorizontal(lipgloss.Top, m.orphans.View(), " ", analysis),
		help,
	)
}

// Run starts the interactive explorer and blocks until the user quits.
func Run(g *graph.Graph, usage map[string]Usage, currentContext string, contexts []string, load Loader, diagnose Diagnoser) error {
	p := tea.NewProgram(New(g, usage, currentContext, contexts, load, diagnose), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// metricLine renders one labeled gauge row: "cpu ████░░ 62%  120m/150m", or a
// no-limit note when no limit is declared.
func metricLine(label string, used, limit int64, format func(int64) string) string {
	if limit <= 0 {
		return fmt.Sprintf("%s %s %s\n",
			keyStyle.Render(label), format(used), faint.Render("(no limit set)"))
	}
	pct := int(used * 100 / limit)
	return fmt.Sprintf("%s %s  %s\n",
		keyStyle.Render(label), gauge(pct, 8),
		faint.Render(format(used)+"/"+format(limit)))
}

func fmtMilli(m int64) string {
	if m == 0 {
		return "0"
	}
	if m < 1000 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%.2f", float64(m)/1000)
}

func fmtBytes(b int64) string {
	const (
		mi = 1 << 20
		gi = 1 << 30
	)
	switch {
	case b == 0:
		return "0"
	case b >= gi:
		return fmt.Sprintf("%.1fGi", float64(b)/gi)
	default:
		return fmt.Sprintf("%dMi", b/mi)
	}
}

// badge annotates Pod nodes with a compact health glyph for the graph panel.
func badge(n graph.Node) string {
	if n.Kind != "Pod" {
		return ""
	}
	ok, reason := health.Pod(n)
	if ok {
		return okStyle.Render("● ready")
	}
	return badStyle.Render("✖ " + reason)
}

func nodeItems(g *graph.Graph, kindFilter, namespace string) []list.Item {
	nodes := g.Nodes()
	sort.Slice(nodes, func(i, j int) bool {
		oi, oj := kindOrder(nodes[i].Kind), kindOrder(nodes[j].Kind)
		if oi != oj {
			return oi < oj
		}
		if nodes[i].Namespace != nodes[j].Namespace {
			return nodes[i].Namespace < nodes[j].Namespace
		}
		return nodes[i].Name < nodes[j].Name
	})
	items := make([]list.Item, 0, len(nodes))
	for _, n := range nodes {
		if (kindFilter != "" && n.Kind != kindFilter) || (namespace != "" && n.Namespace != namespace) {
			continue
		}
		items = append(items, nodeItem{node: n})
	}
	return items
}

func problemItems(g *graph.Graph, namespace string) []list.Item {
	items := make([]problemItem, 0)
	for _, node := range g.Nodes() {
		if namespace != "" && node.Namespace != namespace {
			continue
		}
		switch node.Kind {
		case "Pod":
			if healthy, reason := health.Pod(node); !healthy {
				items = append(items, problemItem{node: node, reason: reason})
			}
		case "PersistentVolumeClaim":
			if pvc, ok := node.Raw.(*corev1.PersistentVolumeClaim); ok && pvc != nil {
				if pvc.Status.Phase == corev1.ClaimPending || pvc.Status.Phase == corev1.ClaimLost {
					items = append(items, problemItem{node: node, reason: string(pvc.Status.Phase)})
				}
			}
		case "Service":
			if svc, ok := node.Raw.(*corev1.Service); ok && svc != nil && len(svc.Spec.Selector) > 0 {
				hasEndpoint := false
				for _, e := range g.OutEdges(node.UID) {
					if e.Rel == graph.RelSelects {
						hasEndpoint = true
						break
					}
				}
				if !hasEndpoint {
					items = append(items, problemItem{node: node, reason: "no matching Pods"})
				}
			}
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].node.Namespace != items[j].node.Namespace {
			return items[i].node.Namespace < items[j].node.Namespace
		}
		return items[i].node.Name < items[j].node.Name
	})

	result := make([]list.Item, len(items))
	for index, item := range items {
		result[index] = item
	}
	return result
}

func namespaceItems(g *graph.Graph) []list.Item {
	namespaces := map[string]struct{}{}
	for _, n := range g.Nodes() {
		if n.Namespace != "" {
			namespaces[n.Namespace] = struct{}{}
		}
	}

	names := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		names = append(names, namespace)
	}
	sort.Strings(names)

	items := make([]list.Item, 0, len(names)+1)
	items = append(items, namespaceItem{})
	for _, namespace := range names {
		items = append(items, namespaceItem{namespace: namespace})
	}
	return items
}

func namespaceItemIndex(items []list.Item, namespace string) int {
	for index, item := range items {
		if item.(namespaceItem).namespace == namespace {
			return index
		}
	}
	return 0
}

func ctxItems(contexts []string) []list.Item {
	items := make([]list.Item, 0, len(contexts))
	for _, c := range contexts {
		items = append(items, ctxItem(c))
	}
	return items
}

func kindOrder(kind string) int {
	order := map[string]int{
		"Deployment": 0, "StatefulSet": 1, "DaemonSet": 2, "ReplicaSet": 3,
		"CronJob": 4, "Job": 5, "Service": 6, "Ingress": 7, "Pod": 8,
		"ConfigMap": 9, "Secret": 10, "PersistentVolumeClaim": 11,
		"PersistentVolume": 12, "ServiceAccount": 13, "Node": 14,
	}
	if o, ok := order[kind]; ok {
		return o
	}
	return 99
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func truncate(s string, max int) string {
	if max <= 1 || len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

// nodeItem adapts a graph node to the bubbles list interface.
type nodeItem struct{ node graph.Node }

func (i nodeItem) Title() string       { return i.node.Kind + " " + i.node.Name }
func (i nodeItem) Description() string { return i.node.Namespace }
func (i nodeItem) FilterValue() string { return i.node.Kind + "/" + i.node.Name }

type problemItem struct {
	node   graph.Node
	reason string
}

func (i problemItem) Title() string       { return i.node.Kind + " " + i.node.Namespace + "/" + i.node.Name }
func (i problemItem) Description() string { return i.reason }
func (i problemItem) FilterValue() string { return i.Title() }

// orphanItem adapts an orphaned graph node to the bubbles list interface.
type orphanItem struct {
	node   graph.Node
	reason string
}

func (i orphanItem) Title() string       { return i.node.Kind + " " + i.node.Namespace + "/" + i.node.Name }
func (i orphanItem) Description() string { return i.reason }
func (i orphanItem) FilterValue() string { return i.Title() }

func orphanItems(g *graph.Graph, namespace string) []list.Item {
	orphans := query.Orphaned(g, namespace)
	items := make([]list.Item, len(orphans))
	for idx, o := range orphans {
		items[idx] = orphanItem{node: o.Node, reason: o.Reason}
	}
	return items
}

// namespaceItem adapts a namespace to the bubbles list interface. An empty
// namespace represents the explicit all-namespaces option.
type namespaceItem struct{ namespace string }

func (i namespaceItem) Title() string {
	if i.namespace == "" {
		return "All namespaces"
	}
	return i.namespace
}

func (i namespaceItem) Description() string {
	if i.namespace == "" {
		return "Show resources from every namespace"
	}
	return "Show resources in this namespace"
}

func (i namespaceItem) FilterValue() string { return i.Title() }

// ctxItem adapts a context name to the bubbles list interface.
type ctxItem string

func (c ctxItem) Title() string       { return string(c) }
func (c ctxItem) Description() string { return "" }
func (c ctxItem) FilterValue() string { return string(c) }

// resourceDelegate renders each resource on a single dense, colored line, with
// a bottleneck marker for pods at/over their limit.
type resourceDelegate struct {
	usage map[string]Usage
}

func (resourceDelegate) Height() int                         { return 1 }
func (resourceDelegate) Spacing() int                        { return 0 }
func (resourceDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d resourceDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(nodeItem)
	if !ok {
		return
	}
	selected := index == m.Index()

	marker := " "
	if it.node.Kind == "Pod" {
		if u, ok := d.usage[it.node.Namespace+"/"+it.node.Name]; ok {
			if overThreshold(u.CPUMilli, u.CPULimitMilli, 90) || overThreshold(u.MemBytes, u.MemLimitBytes, 90) {
				marker = badStyle.Render("▲")
			} else if overThreshold(u.CPUMilli, u.CPULimitMilli, 75) || overThreshold(u.MemBytes, u.MemLimitBytes, 75) {
				marker = warnStyle.Render("▲")
			}
		}
	}

	icon := renderer.Icon(it.node.Kind)
	name := truncate(it.node.Name, m.Width()-7)
	if selected {
		name = lipgloss.NewStyle().Bold(true).Render(name)
	}
	if c, ok := renderer.KindColor(it.node.Kind); ok {
		icon = lipgloss.NewStyle().Foreground(c).Render(icon)
	}

	cursor := "  "
	if selected {
		cursor = cursorStyle.Render("▸ ")
	}
	fmt.Fprint(w, cursor+marker+icon+" "+name)
}
