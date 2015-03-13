package api

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CenturyLinkLabs/panamax-remote-agent-go/agent"
	"github.com/stretchr/testify/assert"
)

var (
	server        *httptest.Server
	adapterServer *httptest.Server
	baseURI       string
	router        http.Handler
	dRepo         agent.DeploymentRepo
)

func init() {
	dRepo = agent.NewDeploymentRepo("../db/agent_test.db")
}

func setup(dunk http.Handler) {
	adapterServer = httptest.NewServer(dunk)
	ad := agent.NewAdapter(adapterServer.URL)
	dm, err := agent.NewDeploymentManager(dRepo, ad)
	if err != nil {
		fmt.Println(err)
	}

	router = NewServer(dm).newRouter()
	server = httptest.NewServer(router)
	baseURI = server.URL
}

func teardown() {
	server.Close()
}

func doGET(url string) (*http.Response, error) {
	c := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	req.Header.Add("Accept", "application/json")

	res, errr := c.Do(req)
	if errr != nil {
		fmt.Println(errr)
		return nil, errr
	}

	return res, nil
}

func doPOST(url string, r io.Reader) (*http.Response, error) {
	c := &http.Client{}

	req, err := http.NewRequest("POST", url, r)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	res, errr := c.Do(req)
	if errr != nil {
		fmt.Println(errr)
		return nil, errr
	}

	return res, nil
}

func doDELETE(url string) (*http.Response, error) {
	c := &http.Client{}

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	res, errr := c.Do(req)
	if errr != nil {
		fmt.Println(errr)
		return nil, errr
	}

	return res, nil
}

func removeAll() {
	res, _ := doGET(baseURI + "/deployments")
	defer res.Body.Close()

	drs := &[]agent.DeploymentResponseLite{}
	jd := json.NewDecoder(res.Body)
	if err := jd.Decode(drs); err != nil {
		panic(err)
	}

	for _, dr := range *drs {
		url := fmt.Sprintf("%s/deployments/%s", baseURI, dr.ID)
		doDELETE(url)
	}
}

func TestListDeploymentsWhenNoDeploymentsExist(t *testing.T) {
	setup(nil)
	defer teardown()
	removeAll()

	res, _ := doGET(baseURI + "/deployments")
	defer res.Body.Close()

	body, _ := ioutil.ReadAll(res.Body)

	assert.Equal(t, "[]", strings.TrimSpace(string(body)))
	assert.Equal(t, 200, res.StatusCode)
	assert.Equal(t, "application/json; charset=utf-8", res.Header["Content-Type"][0])
}

func TestCreateDeployment(t *testing.T) {
	setup(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jd := json.NewDecoder(r.Body)
		imgs := &[]agent.Image{}
		if err := jd.Decode(imgs); err != nil {
			panic(err)
		}

		assert.Equal(t, []agent.Image{
			{
				Name:    "wp",
				Source:  "ov_source",
				Command: "./run.sh",
				Links: []agent.Link{
					{Service: "mysql", Alias: "DB_1"},
				},
				Ports: []agent.Port{
					agent.Port{HostPort: 1111, ContainerPort: 80},
				},
				Environment: []agent.Environment{
					{Variable: "DB_PASSWORD", Value: "pass@word02"},
					{Variable: "DB_NAME", Value: "wordpress"},
				},
				Deployment: agent.DeploymentSettings{Count: 1},
			},
			{
				Name:    "mysql",
				Source:  "centurylink/mysql:5.5",
				Command: "./run_better.sh",
				Ports: []agent.Port{
					{HostPort: 3306, ContainerPort: 3306},
				},
				Environment: []agent.Environment{
					{Variable: "MYSQL_ROOT_PASSWORD", Value: "pass@word02"},
				},
				Deployment: agent.DeploymentSettings{Count: 0},
			},
		}, *imgs)

	}))
	defer teardown()

	buf := strings.NewReader(`{
		"override":{
			"images":[
				{
					"name":"wp",
					"source":"ov_source",
					"environment":[
						{ "variable":"DB_PASSWORD", "value":"pass@word02" }
					],
					"ports":[
						{ "host_port": 1111, "container_port":80 }
					],
					"deployment":{ "count":1 }
				},
				{
					"name":"mysql",
					"environment":[
						{ "variable":"MYSQL_ROOT_PASSWORD", "value":"pass@word02" }
					],
					"command":"./run_better.sh"
				}
			]
		},
		"template":{
			"name": "fooya",
			"images":[
				{
					"name":"wp",
					"source":"centurylink/wordpress:3.9.1",
					"links":[
						{ "service":"mysql", "alias":"DB_1" }
					],
					"ports":[
						{ "host_port":8000, "container_port":80 }
					],
					"environment":[
						{ "variable":"DB_PASSWORD", "value":"pass@word01" },
						{ "variable":"DB_NAME", "value":"wordpress" }
					],
					"command":"./run.sh"
				},
				{
					"name":"mysql",
					"source":"centurylink/mysql:5.5",
					"environment":[
						{ "variable":"MYSQL_ROOT_PASSWORD", "value":"pass@word01" }
					],
					"ports":[
						{ "host_port":3306, "container_port":3306 }
					],
					"command":"./run.sh"
				}
			]
		}
	}`)

	res, err := doPOST(baseURI+"/deployments", buf)
	if err != nil {
		fmt.Println(err)
	}
	defer res.Body.Close()

	jd := json.NewDecoder(res.Body)
	dr := &agent.DeploymentResponseLite{}
	if err := jd.Decode(dr); err != nil {
		panic(err)
	}

	assert.Equal(t, 201, res.StatusCode)
	assert.Equal(t, "application/json; charset=utf-8", res.Header["Content-Type"][0])
	assert.NotNil(t, dr.ID)
	assert.Equal(t, "fooya", dr.Name)
	assert.Equal(t, true, dr.Redeployable)
	// TODO see below for adapter stuff
	// assert.Equal(t, []string{"db-1", "wp-pod", "mysql-pod"}, dr.ServiceIDs)
}

func TestListDeploymentsWhenOneExists(t *testing.T) {
	setup(nil)
	defer teardown()

	res, _ := doGET(baseURI + "/deployments")
	defer res.Body.Close()

	drs := make(agent.DeploymentResponses, 0)
	jd := json.NewDecoder(res.Body)
	if err := jd.Decode(&drs); err != nil {
		panic(err)
	}

	dr := drs[0]

	assert.Equal(t, 200, res.StatusCode)
	assert.Equal(t, "application/json; charset=utf-8", res.Header["Content-Type"][0])
	assert.Equal(t, 1, len(drs))
	assert.Equal(t, "fooya", dr.Name)
	assert.Equal(t, true, dr.Redeployable)
	// TODO see below for adapter stuff
	// assert.Equal(t, []string{"db-1", "wp-pod", "mysql-pod"}, dr.ServiceIDs)
}

func TestGetDeployment(t *testing.T) {
	setup(nil)
	defer teardown()

	res, _ := doGET(baseURI + "/deployments")
	defer res.Body.Close()
	drs := make(agent.DeploymentResponses, 0)
	jd := json.NewDecoder(res.Body)
	if err := jd.Decode(&drs); err != nil {
		panic(err)
	}

	resp, _ := doGET(fmt.Sprintf("%v/deployments/%s", baseURI, drs[0].ID))
	defer resp.Body.Close()

	dr := &agent.DeploymentResponseFull{}
	jdd := json.NewDecoder(resp.Body)
	if err := jdd.Decode(dr); err != nil {
		panic(err)
	}

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/json; charset=utf-8", resp.Header["Content-Type"][0])
	assert.Equal(t, "fooya", dr.Name)
	assert.Equal(t, true, dr.Redeployable)
	// TODO: test all this once we have the fake adapter
	// assert.Equal(t, 3, len(dr.Status.Services))

	// sis := make([]string, 0)
	// sas := make([]string, 0)
	// for _, s := range dr.Status.Services {
	//   sis = append(sis, s.ID)
	//   sas = append(sas, s.ActualState)
	// }

	// assert.Equal(t, []string{"not found", "Running", "Waiting"}, sas)
}

func TestDeleteDeployment(t *testing.T) {
	setup(nil)
	defer teardown()

	res, _ := doGET(baseURI + "/deployments")
	defer res.Body.Close()
	drs := make(agent.DeploymentResponses, 0)
	jd := json.NewDecoder(res.Body)
	if err := jd.Decode(&drs); err != nil {
		panic(err)
	}
	assert.Equal(t, 1, len(drs))

	url := fmt.Sprintf("%s/deployments/%s", baseURI, drs[0].ID)
	doDELETE(url)

	resp, _ := doGET(baseURI + "/deployments")
	body, _ := ioutil.ReadAll(resp.Body)
	defer res.Body.Close()
	assert.Equal(t, "[]", strings.TrimSpace(string(body)))
}

// func TestReDeploy(t *testing.T) {
//   res, _ := doGET(baseURI + "/deployments")
//   defer res.Body.Close()
//   drs := make(DeploymentResponses, 0)
//   jd := json.NewDecoder(res.Body)
//   if err := jd.Decode(&drs); err != nil {
//     panic(err)
//   }

//   resp, _ := doPOST(fmt.Sprintf("%v/deployments/%d/redeploy", baseURI, drs[0].ID), &bytes.Buffer{})
//   defer resp.Body.Close()

//   dr := &DeploymentResponseLite{}
//   jdd := json.NewDecoder(resp.Body)
//   if err := jdd.Decode(dr); err != nil {
//     panic(err)
//   }

//   assert.Equal(t, 201, resp.StatusCode)
//   assert.Equal(t, "application/json; charset=utf-8", resp.Header["Content-Type"][0])
//   assert.Equal(t, "foo", dr.Name)
//   assert.Equal(t, true, dr.Redeployable)
//   assert.Equal(t, 3, len(dr.ServiceIDs))
// }
