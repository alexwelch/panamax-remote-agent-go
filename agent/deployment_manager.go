package agent

import (
	"bytes"
	"encoding/json"

	"github.com/CenturyLinkLabs/panamax-remote-agent-go/adapter"
	"github.com/CenturyLinkLabs/panamax-remote-agent-go/repo"
)

type deploymentManager struct {
	Repo   repo.Persister
	Client adapter.Client
}

// MakeDeploymentManager returns a deploymentManager hydrated with a persister and adapter client.
func MakeDeploymentManager(p repo.Persister, c adapter.Client) Manager {
	return deploymentManager{
		Repo:   p,
		Client: c,
	}
}

// ListDeployments lists all available deployments in a repo.
func (dm deploymentManager) ListDeployments() ([]DeploymentResponseLite, error) {
	deps, err := dm.Repo.All()
	if err != nil {
		return []DeploymentResponseLite{}, err
	}

	drs := make([]DeploymentResponseLite, len(deps), len(deps))

	for i, dep := range deps {
		dr := deploymentResponseLiteFromRawValues(
			dep.ID,
			dep.Name,
			dep.Template,
			dep.ServiceIDs,
		)

		drs[i] = dr
	}

	return drs, nil
}

// GetFullDeployment returns an extended representation of the deployment with the given ID.
func (dm deploymentManager) GetFullDeployment(qid int) (DeploymentResponseFull, error) {
	dep, err := dm.GetDeployment(qid)

	if err != nil {
		return DeploymentResponseFull{}, err
	}

	as := make([]Service, len(dep.ServiceIDs), len(dep.ServiceIDs))
	for i, sID := range dep.ServiceIDs {
		srvc, err := dm.Client.GetService(sID)
		if err != nil {
			return DeploymentResponseFull{}, err
		}
		as[i] = Service{
			ID:          srvc.ID,
			ActualState: srvc.ActualState,
		}
	}

	dr := DeploymentResponseFull{
		Name:         dep.Name,
		ID:           dep.ID,
		Redeployable: dep.Redeployable,
		Status:       Status{Services: as},
	}

	return dr, nil
}

// GetDeployment returns a representation of the deployment with the given ID.
func (dm deploymentManager) GetDeployment(qid int) (DeploymentResponseLite, error) {
	dep, err := dm.Repo.FindByID(qid)
	if err != nil {
		return DeploymentResponseLite{}, err
	}

	drl := deploymentResponseLiteFromRawValues(
		dep.ID,
		dep.Name,
		dep.Template,
		dep.ServiceIDs,
	)

	return drl, nil
}

// DeleteDeployment deletes the deployment, with the given ID,
// from both the repo and adapter.
func (dm deploymentManager) DeleteDeployment(qID int) error {
	dep, err := dm.Repo.FindByID(qID)

	if err != nil {
		return err
	}

	var sIDs []string
	if err := json.Unmarshal([]byte(dep.ServiceIDs), &sIDs); err != nil {
		return err
	}

	for _, sID := range sIDs {
		if err := dm.Client.DeleteService(sID); err != nil {
			return err
		}
	}

	if err := dm.Repo.Remove(qID); err != nil {
		return err
	}

	return err
}

// CreateDeployment creates a new deployment from a DeploymentBlueprint.
func (dm deploymentManager) CreateDeployment(depB DeploymentBlueprint) (DeploymentResponseLite, error) {

	mImgs := depB.MergedImages()

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(mImgs); err != nil {
		return DeploymentResponseLite{}, err
	}

	as, err := dm.Client.CreateServices(buf)
	if err != nil {
		return DeploymentResponseLite{}, err
	}

	tn := depB.Template.Name
	dep, err := makeRepoDeployment(tn, mImgs, as)
	if err != nil {
		return DeploymentResponseLite{}, err
	}

	if err := dm.Repo.Save(&dep); err != nil {
		return DeploymentResponseLite{}, err
	}

	drl := deploymentResponseLiteFromRawValues(
		dep.ID,
		dep.Name,
		dep.Template,
		dep.ServiceIDs,
	)

	return drl, nil
}

// ReDeploy recreates a given deployment, by deleteing, then creating with the
// same template. The returned record will have a new ID.
func (dm deploymentManager) ReDeploy(ID int) (DeploymentResponseLite, error) {

	dep, err := dm.Repo.FindByID(ID)

	var tpl Template
	if err := json.Unmarshal([]byte(dep.Template), &tpl); err != nil {
		return DeploymentResponseLite{}, err
	}

	if err := dm.DeleteDeployment(ID); err != nil {
		return DeploymentResponseLite{}, err
	}

	drl, err := dm.CreateDeployment(DeploymentBlueprint{Template: tpl})
	if err != nil {
		return DeploymentResponseLite{}, err
	}

	return drl, nil
}

// FetchMetadata returns metadata for the agent and adapter.
func (dm deploymentManager) FetchMetadata() (Metadata, error) {
	adapterMeta, _ := dm.Client.FetchMetadata()

	md := Metadata{
		Agent: struct {
			Version string `json:"version"`
		}{Version: "v1"}, // TODO pull this from a const or ENV or something
		Adapter: adapterMeta,
	}

	return md, nil
}

func makeRepoDeployment(tn string, mImgs []Image, as []adapter.Service) (repo.Deployment, error) {
	ts, err := stringifyTemplate(tn, mImgs)
	if err != nil {
		return repo.Deployment{}, err
	}

	ss, err := stringifyServiceIDs(as)

	if err != nil {
		return repo.Deployment{}, err
	}

	return repo.Deployment{
		Name:       tn,
		Template:   ts,
		ServiceIDs: ss,
	}, nil
}

func stringifyTemplate(tn string, imgs []Image) (string, error) {
	mt := Template{
		Name:   tn,
		Images: imgs,
	}
	b, err := json.Marshal(mt)

	return string(b), err
}

func stringifyServiceIDs(as []adapter.Service) (string, error) {
	sIDs := make([]string, len(as), len(as))

	for i, ar := range as {
		sIDs[i] = ar.ID
	}

	sb, err := json.Marshal(sIDs)

	return string(sb), err
}

func deploymentResponseLiteFromRawValues(id int, nm string, tpl string, sids string) DeploymentResponseLite {
	drl := &DeploymentResponseLite{
		ID:           id,
		Name:         nm,
		Redeployable: tpl != "",
	}
	// if this is an empty string it will fail to unmarshal, and we are fine with that.
	json.Unmarshal([]byte(sids), &drl.ServiceIDs)

	return *drl
}
