package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type Adapter struct {
	Endpoint string
	Client   *http.Client
}

func NewAdapter(ep string) Adapter {
	client := &http.Client{}

	ad := Adapter{
		Client:   client,
		Endpoint: ep,
	}

	return ad
}

func (ad *Adapter) CreateServices(sIDs []Image) AdapterResponses {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(sIDs); err != nil {
		panic(err)
	}
	resp, _ := ad.Client.Post(ad.servicesPath(""), "application/json", buf)

	ars := &AdapterResponses{}
	jd := json.NewDecoder(resp.Body)
	jd.Decode(ars)

	return *ars
}

func (ad *Adapter) servicesPath(id string) string {
	return ad.Endpoint + "/v1/services/" + id
}

type AdapterResponses []AdapterResponse

type AdapterResponse struct {
	ID          string `json:"id"`
	ActualState string `json:"actualState,omitempty"`
}
