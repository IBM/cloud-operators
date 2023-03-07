module github.com/ibm/cloud-operators

go 1.14

require (
	github.com/IBM-Cloud/bluemix-go v0.0.0-20200716122208-488c9de67b8c
	github.com/blang/semver/v4 v4.0.0
	github.com/ghodss/yaml v1.0.0
	github.com/go-git/go-git/v5 v5.1.0
	github.com/go-logr/logr v0.1.0
	github.com/go-logr/zapr v0.1.0
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/johnstarich/go/regext v0.0.1
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/pkg/errors v0.8.1
	github.com/softlayer/softlayer-go v1.1.2 // indirect
	github.com/stretchr/testify v1.6.1
	go.uber.org/zap v1.10.0
	golang.org/x/net v0.7.0 // indirect
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.17.17
	k8s.io/apiextensions-apiserver v0.17.17
	k8s.io/apimachinery v0.17.17
	k8s.io/client-go v0.17.17
	sigs.k8s.io/controller-runtime v0.5.1
)

replace github.com/dgrijalva/jwt-go => github.com/form3tech-oss/jwt-go v3.2.1+incompatible // FIXME: https://github.com/dgrijalva/jwt-go/issues/463
