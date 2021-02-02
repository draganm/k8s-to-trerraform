package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

type meta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"metadata"`
}

func (m meta) resourceName() string {

	name := m.Metadata.Name

	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, ".", "_")

	resourceName := fmt.Sprintf("%s_%s", strings.ToLower(m.Kind), name)

	if m.Metadata.Namespace != "" {
		resourceName = fmt.Sprintf("%s_%s_%s", strings.ToLower(m.Kind), m.Metadata.Namespace, name)
	}

	return resourceName
}

// {
// 	"resource": {
// 	  "aws_instance": {
// 		"example": {
// 		  "instance_type": "t2.micro",
// 		  "ami": "ami-abc123"
// 		}
// 	  }
// 	}
//   }

// resource "kubernetes_manifest" "test-configmap" {
// 	provider = kubernetes-alpha

// 	manifest = {
// 	  "apiVersion" = "v1"
// 	  "kind" = "ConfigMap"
// 	  "metadata" = {
// 		"name" = "test-config"
// 		"namespace" = "default"
// 	  }
// 	  "data" = {
// 		"foo" = "bar"
// 	  }
// 	}
//   }

func makeTFManifest(m map[string]interface{}, prevName string) (string, interface{}, error) {

	d, err := json.Marshal(m)
	if err != nil {
		return "", nil, errors.Wrap(err, "while marshalling JSON")
	}

	mh := &meta{}

	err = json.Unmarshal(d, &mh)
	if err != nil {
		return "", nil, errors.Wrap(err, "while unmarshalling meta header")
	}

	delete(m, "status")

	res := map[string]interface{}{
		"manifest": m,
		"provider": "kubernetes-alpha",
	}

	if prevName != "" {
		res["depends_on"] = []string{
			fmt.Sprintf("kubernetes_manifest.%s", prevName),
		}
	}

	resMap := map[string]interface{}{}
	resMap[mh.resourceName()] = res

	return mh.resourceName(), map[string]interface{}{
		"resource": map[string]interface{}{
			"kubernetes_manifest": resMap,
		},
	}, nil
}

func main() {

	app := &cli.App{
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return errors.New("yaml file must be provided")
			}

			yf, err := os.Open(c.Args().First())
			if err != nil {
				return errors.Wrap(err, "while opening config file")
			}

			defer yf.Close()

			dec := yaml.NewDecoder(yf)

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")

			prevName := ""

			for i := 0; ; i++ {

				obj := map[string]interface{}{}

				err := dec.Decode(&obj)
				if err == io.EOF {
					break
				}

				if err != nil {
					return errors.Wrap(err, "while parsing yaml stream")
				}

				resourceName, tfm, err := makeTFManifest(obj, "")
				if err != nil {
					return errors.Wrap(err, "while creating TF manifest")
				}

				prevName = resourceName

				fileName := fmt.Sprintf("%03d-%s.tf.json", i, prevName)

				d, err := json.MarshalIndent(tfm, "", "  ")
				if err != nil {
					return errors.Wrapf(err, "while marshalling terraform of %s to JSON", resourceName)
				}

				err = ioutil.WriteFile(fileName, d, 0700)

				if err != nil {
					return errors.Wrapf(err, "while writing %s", fileName)
				}

				// err = enc.Encode(tfm)
				// if err != nil {
				// 	return errors.Wrap(err, "while encoding JSON")
				// }
			}

			return nil
		},
	}

	app.RunAndExitOnError()

}
