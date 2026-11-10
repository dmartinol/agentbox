package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dmartinol/agentbox/internal/app"
	agentboxweb "github.com/dmartinol/agentbox/internal/gui/web"
)

type Server struct {
	app   *app.App
	tasks *taskManager
}

func New(a *app.App) (*Server, error) {
	if a == nil {
		return nil, fmt.Errorf("application is required")
	}
	return &Server{app: a, tasks: newTaskManager()}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("POST /api/runs", s.startRun)
	mux.HandleFunc("GET /api/runs/{id}", s.getRun)
	mux.HandleFunc("POST /api/runs/{id}/stop", s.stopRun)
	mux.HandleFunc("POST /api/runs/{id}/sync", s.syncRun)
	mux.HandleFunc("POST /api/runs/{id}/delete", s.deleteRun)
	mux.HandleFunc("DELETE /api/runs/{id}", s.deleteRun)
	mux.HandleFunc("GET /api/runs/{id}/logs", s.runLogs)
	mux.HandleFunc("GET /api/runs/{id}/diff", s.runDiff)
	mux.HandleFunc("GET /api/runs/{id}/policy", s.runPolicy)

	mux.HandleFunc("GET /api/projects", s.listProjects)
	mux.HandleFunc("POST /api/projects", s.saveProject)
	mux.HandleFunc("GET /api/profiles", s.listProfiles)
	mux.HandleFunc("POST /api/profiles", s.saveProfile)
	mux.HandleFunc("GET /api/providers", s.listProviders)
	mux.HandleFunc("POST /api/providers", s.setupProvider)
	mux.HandleFunc("POST /api/providers/{name}/update", s.updateProvider)
	mux.HandleFunc("POST /api/providers/{name}/delete", s.deleteProvider)

	mux.HandleFunc("GET /api/images", s.listImages)
	mux.HandleFunc("POST /api/images/version", s.imageVersion)
	mux.HandleFunc("POST /api/images/build", s.buildImage)
	mux.HandleFunc("GET /api/tasks/{id}", s.getTask)
	mux.HandleFunc("POST /api/tasks/{id}/stop", s.stopTask)
	mux.HandleFunc("POST /api/images/pull", s.pullImage)
	mux.HandleFunc("POST /api/images/push", s.pushImage)
	mux.HandleFunc("GET /api/registry", s.getRegistry)
	mux.HandleFunc("POST /api/registry", s.saveRegistry)
	mux.HandleFunc("POST /api/registry/discover", s.discoverRegistry)

	mux.HandleFunc("GET /api/gateways", s.listGateways)
	mux.HandleFunc("POST /api/gateways/connect", s.connectGateway)
	mux.HandleFunc("POST /api/gateways/start", s.startGateway)
	mux.HandleFunc("GET /api/gateways/{name}/info", s.gatewayInfo)

	mux.HandleFunc("GET /api/policies", s.listPolicies)
	mux.HandleFunc("POST /api/policies", s.createPolicy)
	mux.HandleFunc("GET /api/policies/{name}", s.getPolicyContent)
	mux.HandleFunc("POST /api/policies/{name}", s.savePolicyContent)
	mux.HandleFunc("POST /api/policies/{name}/host", s.addPolicyHost)
	mux.HandleFunc("POST /api/policies/{name}/apply", s.applyPolicy)

	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("POST /api/settings", s.saveSettings)
	mux.HandleFunc("GET /api/doctor", s.doctor)

	mux.Handle("/", spaHandler())
	return mux
}

func spaHandler() http.Handler {
	assets, err := fs.Sub(agentboxweb.Assets(), "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(assets, path); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, context.Canceled) {
		status = http.StatusRequestTimeout
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request: " + err.Error()})
		return false
	}
	return true
}

type gatewayRecord struct {
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	Endpoint string `json:"endpoint"`
	Type     string `json:"type"`
	Auth     string `json:"auth"`
	Source   string `json:"source"`
}

func (s *Server) gateways(ctx context.Context) ([]gatewayRecord, error) {
	data, err := s.app.GatewayList(ctx)
	if err != nil {
		return nil, err
	}
	var rows []gatewayRecord
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("invalid gateway JSON: %w", err)
	}
	return rows, nil
}

type projectRunNode struct {
	Project      string    `json:"project"`
	Root         string    `json:"root"`
	Isolation    string    `json:"isolation"`
	ActiveRuns   []app.Run `json:"activeRuns"`
	IdleRuns     []app.Run `json:"idleRuns"`
	FinishedRuns []app.Run `json:"finishedRuns"`
}

type overviewResponse struct {
	Summary  map[string]int      `json:"summary"`
	Tree     []projectRunNode    `json:"tree"`
	Runs     []app.Run           `json:"runs"`
	Projects []app.Project       `json:"projects"`
	Profiles []app.Profile       `json:"profiles"`
	Images   []app.ImageArtifact `json:"images"`
	Gateways []gatewayRecord     `json:"gateways"`
	Doctor   []app.Check         `json:"doctor"`
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	runs, err := s.app.ListAndReconcile(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	projects, err := s.app.Store.ListProjects()
	if err != nil {
		writeError(w, err)
		return
	}
	profiles, err := s.app.Store.ListProfiles()
	if err != nil {
		writeError(w, err)
		return
	}
	images, err := s.app.LocalImages(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	gateways, err := s.gateways(ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	doctor := s.app.Doctor(ctx)
	tree := newProjectRunTree(projects, runs)
	writeJSON(w, http.StatusOK, overviewResponse{
		Summary:  newSummary(runs, images, gateways, profiles, projects),
		Tree:     tree,
		Runs:     runs,
		Projects: projects,
		Profiles: profiles,
		Images:   images,
		Gateways: gateways,
		Doctor:   doctor,
	})
}

func newProjectRunTree(projects []app.Project, runs []app.Run) []projectRunNode {
	roots := map[string]string{}
	isolation := map[string]string{}
	names := make([]string, 0, len(projects))
	for _, project := range projects {
		roots[project.Name] = project.Root
		mode := "direct"
		if project.UseWorktree() {
			mode = "worktree"
		}
		isolation[project.Name] = mode
		names = append(names, project.Name)
	}
	for _, run := range runs {
		name := run.Project
		if name == "" {
			name = "unassigned"
		}
		if _, ok := roots[name]; !ok {
			roots[name] = run.Source
			isolation[name] = map[bool]string{true: "worktree", false: "direct"}[run.Isolated()]
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]projectRunNode, 0, len(names))
	for _, name := range names {
		node := projectRunNode{Project: name, Root: roots[name], Isolation: isolation[name], ActiveRuns: []app.Run{}, IdleRuns: []app.Run{}, FinishedRuns: []app.Run{}}
		for _, run := range runs {
			runProject := run.Project
			if runProject == "" {
				runProject = "unassigned"
			}
			if runProject != name {
				continue
			}
			switch status := strings.ToLower(run.Status); {
			case status == "running" || status == "created" || status == "prepared":
				node.ActiveRuns = append(node.ActiveRuns, run)
			case status == "finished":
				node.FinishedRuns = append(node.FinishedRuns, run)
			case status != "finished" && status != "running" && status != "created" && status != "prepared":
				node.IdleRuns = append(node.IdleRuns, run)
			}
		}
		out = append(out, node)
	}
	return out
}

func newSummary(runs []app.Run, images []app.ImageArtifact, gateways []gatewayRecord, profiles []app.Profile, projects []app.Project) map[string]int {
	summary := map[string]int{
		"images":   len(images),
		"gateways": len(gateways),
		"profiles": len(profiles),
		"projects": len(projects),
	}
	for _, run := range runs {
		switch status := strings.ToLower(run.Status); {
		case status == "running" || status == "created" || status == "prepared":
			summary["active"]++
		case status == "finished":
			summary["finished"]++
		case status == "error" || status == "create-failed" || status == "policy-blocked" || status == "unreadable" || status == "unreachable":
			summary["failed"]++
		}
		if !run.Synced {
			summary["unsynced"]++
		}
	}
	return summary
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.app.ListAndReconcile(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.app.Store.LoadRun(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

type startRunRequest struct {
	Profile  string `json:"profile"`
	Project  string `json:"project"`
	Name     string `json:"name"`
	Gateway  string `json:"gateway"`
	Image    string `json:"image"`
	Mode     string `json:"mode"`
	Model    string `json:"model"`
	Approval string `json:"approval"`
	Prompt   string `json:"prompt"`
}

func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	var request startRunRequest
	if !decode(w, r, &request) {
		return
	}
	run, err := s.app.Start(r.Context(), app.StartOptions{
		Profile:  request.Profile,
		Project:  request.Project,
		Name:     request.Name,
		Gateway:  request.Gateway,
		Image:    request.Image,
		Mode:     request.Mode,
		Model:    request.Model,
		Approval: request.Approval,
		Prompt:   request.Prompt,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) stopRun(w http.ResponseWriter, r *http.Request) {
	if err := s.app.Stop(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	run, err := s.app.Store.LoadRun(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) syncRun(w http.ResponseWriter, r *http.Request) {
	if err := s.app.Sync(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	run, err := s.app.Store.LoadRun(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) deleteRun(w http.ResponseWriter, r *http.Request) {
	if err := s.app.Delete(r.Context(), r.PathValue("id"), true); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) runLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.app.Logs(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": string(logs)})
}

func (s *Server) runDiff(w http.ResponseWriter, r *http.Request) {
	run, err := s.app.Store.LoadRun(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	diff, err := s.app.Diff(r.Context(), run)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"diff": string(diff)})
}

func (s *Server) runPolicy(w http.ResponseWriter, r *http.Request) {
	policy, err := s.app.PolicyShow(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

type projectRequest struct {
	Name     string `json:"name"`
	Root     string `json:"root"`
	Worktree *bool  `json:"worktree"`
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.app.Store.ListProjects()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) saveProject(w http.ResponseWriter, r *http.Request) {
	var request projectRequest
	if !decode(w, r, &request) {
		return
	}
	worktree := true
	if request.Worktree != nil {
		worktree = *request.Worktree
	}
	project, err := s.app.RegisterProject(r.Context(), request.Name, request.Root, worktree)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.app.Store.ListProfiles()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (s *Server) saveProfile(w http.ResponseWriter, r *http.Request) {
	var profile app.Profile
	if !decode(w, r, &profile) {
		return
	}
	if profile.Version == 0 {
		profile.Version = 1
	}
	if profile.Policy == "" {
		profile.Policy = filepath.Join(s.app.Store.PolicyDir(), profile.Name+".yaml")
		if _, err := os.Stat(profile.Policy); errors.Is(err, os.ErrNotExist) {
			if _, err := s.app.Store.CreatePolicyConfiguration(profile.Name); err != nil {
				writeError(w, err)
				return
			}
		} else if err != nil {
			writeError(w, err)
			return
		}
	}
	if err := s.app.Store.Ensure(); err != nil {
		writeError(w, err)
		return
	}
	if err := s.app.Store.SaveProfile(profile); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

type providerRequest struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Credential string `json:"credential"`
	Gateway    string `json:"gateway"`
}

type providerRecord struct {
	Name           string   `json:"name"`
	UsedBy         []string `json:"used_by,omitempty"`
	Type           string   `json:"type"`
	ProviderType   string   `json:"provider_type"`
	Profile        string   `json:"profile"`
	ProfileID      string   `json:"profile_id"`
	Credential     string   `json:"credential"`
	CredentialKeys []string `json:"credential_keys"`
	Gateway        string   `json:"gateway"`
}

type providerListPayload struct {
	Providers []providerRecord `json:"providers"`
}

func (r providerRecord) normalized(gateway string) providerRecord {
	if r.Type == "" {
		r.Type = r.ProviderType
	}
	if r.Profile == "" {
		r.Profile = r.ProfileID
	}
	if r.Credential == "" && len(r.CredentialKeys) == 1 {
		r.Credential = r.CredentialKeys[0]
	}
	if r.Gateway == "" {
		r.Gateway = gateway
	}
	return r
}

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	gateway := r.URL.Query().Get("gateway")
	data, err := s.app.ProviderList(r.Context(), gateway)
	if err != nil {
		writeError(w, err)
		return
	}
	var payload providerListPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		writeError(w, fmt.Errorf("invalid provider JSON: %w", err))
		return
	}
	profiles, err := s.app.Store.ListProfiles()
	if err != nil {
		writeError(w, err)
		return
	}
	usedBy := map[string][]string{}
	for _, profile := range profiles {
		usedBy[profile.Provider] = append(usedBy[profile.Provider], profile.Name)
	}
	providers := make([]providerRecord, 0, len(payload.Providers))
	for _, provider := range payload.Providers {
		if provider.Name == "" {
			continue
		}
		provider = provider.normalized(gateway)
		users := usedBy[provider.Name]
		sort.Strings(users)
		provider.UsedBy = users
		providers = append(providers, provider)
	}
	writeJSON(w, http.StatusOK, providers)
}

type providerUpdateRequest struct {
	Credential string `json:"credential"`
	Gateway    string `json:"gateway"`
}

func (s *Server) updateProvider(w http.ResponseWriter, r *http.Request) {
	var request providerUpdateRequest
	if !decode(w, r, &request) {
		return
	}
	name := r.PathValue("name")
	if err := s.app.ProviderUpdateCredential(r.Context(), name, request.Credential, request.Gateway); err != nil {
		log.Printf("gui: provider update failed name=%q credential_env=%q gateway=%q: %v", name, request.Credential, request.Gateway, err)
		writeError(w, fmt.Errorf("provider update failed: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) deleteProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	gateway := r.URL.Query().Get("gateway")
	if err := s.app.ProviderDelete(r.Context(), name, gateway); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "is used by profiles") {
			status = http.StatusConflict
		}
		log.Printf("gui: provider delete failed name=%q gateway=%q: %v", name, gateway, err)
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func clarifyProviderSetupError(err error, gateway string) error {
	message := strings.ToLower(err.Error())
	connectFailed := strings.Contains(message, "connection refused") ||
		strings.Contains(message, "tcp connect error") ||
		strings.Contains(message, "client error (connect)")
	if !connectFailed {
		return err
	}
	return fmt.Errorf("OpenShell gateway %q is unreachable. Start Podman with `podman machine start <machine>`, restart OpenShell with `brew services restart openshell`, verify the gateway from the Doctor page, then retry. Underlying error: %w", gateway, err)
}

func (s *Server) setupProvider(w http.ResponseWriter, r *http.Request) {
	var request providerRequest
	if !decode(w, r, &request) {
		return
	}
	if err := s.app.ProviderSetup(r.Context(), request.Kind, request.Name, request.Host, request.Credential, request.Gateway, false, false); err != nil {
		err = clarifyProviderSetupError(err, request.Gateway)
		// Never log the credential value; only its environment variable name is safe.
		log.Printf("gui: provider setup failed kind=%q name=%q host=%q credential_env=%q gateway=%q: %v",
			request.Kind, request.Name, request.Host, request.Credential, request.Gateway, err)
		writeError(w, fmt.Errorf("provider setup failed: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) listImages(w http.ResponseWriter, r *http.Request) {
	images, err := s.app.LocalImages(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, images)
}

type imageRequest struct {
	Agent     string `json:"agent"`
	Version   string `json:"version"`
	Reference string `json:"reference"`
	LocalTag  string `json:"localTag"`
	Remote    string `json:"remote"`
}

func (s *Server) imageVersion(w http.ResponseWriter, r *http.Request) {
	var request imageRequest
	if !decode(w, r, &request) {
		return
	}
	version, err := s.app.ImageVersion(r.Context(), request.Agent)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": version})
}

func (s *Server) buildImage(w http.ResponseWriter, r *http.Request) {
	var request imageRequest
	if !decode(w, r, &request) {
		return
	}
	task, err := s.tasks.start("image-build", func(ctx context.Context, output io.Writer) error {
		record, err := s.app.ImageBuildWithOutput(ctx, request.Agent, request.Version, false, output, output)
		if err == nil {
			_, _ = fmt.Fprintf(output, "Built image: %s\n", record.Tag)
		}
		return err
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.tasks.get(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) stopTask(w http.ResponseWriter, r *http.Request) {
	task, ok := s.tasks.stop(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) pullImage(w http.ResponseWriter, r *http.Request) {
	var request imageRequest
	if !decode(w, r, &request) {
		return
	}
	if err := s.app.ImagePull(r.Context(), request.Reference, false); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) pushImage(w http.ResponseWriter, r *http.Request) {
	var request imageRequest
	if !decode(w, r, &request) {
		return
	}
	if err := s.app.ImagePush(r.Context(), request.LocalTag, request.Remote, false); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) getRegistry(w http.ResponseWriter, r *http.Request) {
	registry, err := s.app.Registry()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, registry)
}

type registryRequest struct {
	Reference string `json:"reference"`
}

func (s *Server) saveRegistry(w http.ResponseWriter, r *http.Request) {
	var request registryRequest
	if !decode(w, r, &request) {
		return
	}
	if err := s.app.SetRegistry(request.Reference); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.Registry{Reference: request.Reference})
}

func (s *Server) discoverRegistry(w http.ResponseWriter, r *http.Request) {
	images, err := s.app.RegistryDiscover(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, images)
}

func (s *Server) listGateways(w http.ResponseWriter, r *http.Request) {
	gateways, err := s.gateways(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gateways)
}

type gatewayConnectRequest struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

func (s *Server) connectGateway(w http.ResponseWriter, r *http.Request) {
	var request gatewayConnectRequest
	if !decode(w, r, &request) {
		return
	}
	if err := s.app.GatewayConnect(r.Context(), request.URL, request.Name, false); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) startGateway(w http.ResponseWriter, r *http.Request) {
	if err := s.app.GatewayStart(r.Context(), false); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) gatewayInfo(w http.ResponseWriter, r *http.Request) {
	data, err := s.app.GatewayInfo(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := s.app.Store.ListPolicyConfigurations()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policies)
}

type policyRequest struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Agent   string `json:"agent"`
	Host    string `json:"host"`
	RunID   string `json:"runId"`
}

func (s *Server) createPolicy(w http.ResponseWriter, r *http.Request) {
	var request policyRequest
	if !decode(w, r, &request) {
		return
	}
	policy, err := s.app.Store.CreatePolicyConfiguration(request.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, policy)
}

func (s *Server) getPolicyContent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	content, err := s.app.Store.LoadPolicyConfigurationContent(name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "content": string(content)})
}

func (s *Server) savePolicyContent(w http.ResponseWriter, r *http.Request) {
	var request policyRequest
	if !decode(w, r, &request) {
		return
	}
	if err := s.app.Store.SavePolicyConfigurationContent(r.PathValue("name"), []byte(request.Content)); err != nil {
		writeError(w, err)
		return
	}
	policy, err := s.app.Store.LoadPolicyConfiguration(r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) addPolicyHost(w http.ResponseWriter, r *http.Request) {
	var request policyRequest
	if !decode(w, r, &request) {
		return
	}
	if err := s.app.AddPolicyNetworkRule(r.PathValue("name"), request.Agent, request.Host); err != nil {
		writeError(w, err)
		return
	}
	policy, err := s.app.Store.LoadPolicyConfiguration(r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) applyPolicy(w http.ResponseWriter, r *http.Request) {
	var request policyRequest
	if !decode(w, r, &request) {
		return
	}
	if err := s.app.ApplyPolicyConfiguration(r.Context(), r.PathValue("name"), request.RunID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.app.EffectiveSettings()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var settings app.Settings
	if !decode(w, r, &settings) {
		return
	}
	saved, err := s.app.SaveGlobalSettings(settings.Worktrees.Root, settings.Worktrees.BranchPrefix)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) doctor(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.app.Doctor(r.Context()))
}

// Run starts the GUI server and blocks until the context is canceled.
func Run(ctx context.Context, a *app.App, address string) error {
	server, err := New(a)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              address,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	shutdown := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			shutdown <- err
			return
		}
		shutdown <- nil
	}()
	fmt.Printf("AgentBox GUI listening on http://%s\n", address)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-shutdown:
		return err
	}
}
