package secret

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/exp/maps"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	apiv1 "kusionstack.io/kusion/pkg/apis/core/v1"
	"kusionstack.io/kusion/pkg/modules"
	"kusionstack.io/kusion/pkg/modules/inputs/workload"
	"kusionstack.io/kusion/pkg/secrets"
)

type secretGenerator struct {
	project         *apiv1.Project
	namespace       string
	secrets         map[string]workload.Secret
	secretStoreSpec *apiv1.SecretStoreSpec
}

func NewSecretGenerator(ctx modules.GeneratorContext) (modules.Generator, error) {
	if len(ctx.Project.Name) == 0 {
		return nil, fmt.Errorf("project name must not be empty")
	}

	var secretMap map[string]workload.Secret
	if ctx.Application.Workload.Service != nil {
		secretMap = ctx.Application.Workload.Service.Secrets
	} else {
		secretMap = ctx.Application.Workload.Job.Secrets
	}

	return &secretGenerator{
		project:         ctx.Project,
		secrets:         secretMap,
		namespace:       ctx.Namespace,
		secretStoreSpec: ctx.SecretStoreSpec,
	}, nil
}

func NewSecretGeneratorFunc(ctx modules.GeneratorContext) modules.NewGeneratorFunc {
	return func() (modules.Generator, error) {
		return NewSecretGenerator(ctx)
	}
}

func (g *secretGenerator) Generate(spec *apiv1.Intent) error {
	if spec.Resources == nil {
		spec.Resources = make(apiv1.Resources, 0)
	}

	for secretName, secretRef := range g.secrets {
		secret, err := g.generateSecret(secretName, secretRef)
		if err != nil {
			return err
		}

		resourceID := modules.KubernetesResourceID(secret.TypeMeta, secret.ObjectMeta)
		err = modules.AppendToIntent(
			apiv1.Kubernetes,
			resourceID,
			spec,
			secret,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// generateSecret generates target secret based on secret type. Most of these secret types are just semantic wrapper
// of native Kubernetes secret types:https://kubernetes.io/docs/concepts/configuration/secret/#secret-types, and more
// detailed usage info can be found in public documentation.
func (g *secretGenerator) generateSecret(secretName string, secretRef workload.Secret) (*v1.Secret, error) {
	switch secretRef.Type {
	case "basic":
		return g.generateBasic(secretName, secretRef)
	case "token":
		return g.generateToken(secretName, secretRef)
	case "opaque":
		return g.generateOpaque(secretName, secretRef)
	case "certificate":
		return g.generateCertificate(secretName, secretRef)
	case "external":
		return g.generateSecretWithExternalProvider(secretName, secretRef)
	default:
		return nil, fmt.Errorf("unrecognized secret type %s", secretRef.Type)
	}
}

// generateBasic generates secret used for basic authentication. The basic secret type
// is used for username / password pairs.
func (g *secretGenerator) generateBasic(secretName string, secretRef workload.Secret) (*v1.Secret, error) {
	secret := initBasicSecret(g.namespace, secretName, v1.SecretTypeBasicAuth, secretRef.Immutable)
	secret.Data = grabData(secretRef.Data, v1.BasicAuthUsernameKey, v1.BasicAuthPasswordKey)

	for _, key := range []string{v1.BasicAuthUsernameKey, v1.BasicAuthPasswordKey} {
		if len(secret.Data[key]) == 0 {
			v := GenerateRandomString(54)
			secret.Data[key] = []byte(v)
		}
	}

	return secret, nil
}

// generateToken generates secret used for password. Token secrets are useful for generating
// a password or secure string used for passwords when the user is already known or not required.
func (g *secretGenerator) generateToken(secretName string, secretRef workload.Secret) (*v1.Secret, error) {
	secret := initBasicSecret(g.namespace, secretName, v1.SecretTypeOpaque, secretRef.Immutable)
	secret.Data = grabData(secretRef.Data, "token")

	if len(secret.Data["token"]) == 0 {
		v := GenerateRandomString(54)
		secret.Data["token"] = []byte(v)
	}

	return secret, nil
}

// generateOpaque generates secret used for arbitrary user-defined data.
func (g *secretGenerator) generateOpaque(secretName string, secretRef workload.Secret) (*v1.Secret, error) {
	secret := initBasicSecret(g.namespace, secretName, v1.SecretTypeOpaque, secretRef.Immutable)
	secret.Data = grabData(secretRef.Data, maps.Keys(secretRef.Data)...)
	return secret, nil
}

// generateCertificate generates secret used for storing a certificate and its associated key.
// One common use for TLS Secrets is to configure encryption in transit for an Ingress, but
// you can also use it with other resources or directly in your workload.
func (g *secretGenerator) generateCertificate(secretName string, secretRef workload.Secret) (*v1.Secret, error) {
	secret := initBasicSecret(g.namespace, secretName, v1.SecretTypeTLS, secretRef.Immutable)
	secret.Data = grabData(secretRef.Data, v1.TLSCertKey, v1.TLSPrivateKeyKey)
	return secret, nil
}

// generateSecretWithExternalProvider retrieves target sensitive information from external secret provider and
// generates corresponding Kubernetes Secret object.
func (g *secretGenerator) generateSecretWithExternalProvider(secretName string, secretRef workload.Secret) (*v1.Secret, error) {
	if g.secretStoreSpec == nil {
		return nil, errors.New("secret store is missing, please add valid secret store spec in workspace")
	}

	secret := initBasicSecret(g.namespace, secretName, v1.SecretTypeOpaque, secretRef.Immutable)
	secret.Data = make(map[string][]byte)

	var allErrs []error
	for key, ref := range secretRef.Data {
		externalSecretRef, err := parseExternalSecretDataRef(ref)
		if err != nil {
			allErrs = append(allErrs, err)
			continue
		}
		provider, exist := secrets.GetProvider(g.secretStoreSpec.Provider)
		if !exist {
			allErrs = append(allErrs, errors.New("no matched secret store found, please check workspace yaml"))
			continue
		}
		secretStore, err := provider.NewSecretStore(*g.secretStoreSpec)
		if err != nil {
			allErrs = append(allErrs, err)
			continue
		}
		secretData, err := secretStore.GetSecret(context.Background(), *externalSecretRef)
		if err != nil {
			allErrs = append(allErrs, err)
			continue
		}
		secret.Data[key] = secretData
	}

	if allErrs != nil {
		return nil, utilerrors.NewAggregate(allErrs)
	}

	return secret, nil
}

// grabData extracts keys mapping data from original string map.
func grabData(from map[string]string, keys ...string) map[string][]byte {
	to := map[string][]byte{}
	for _, key := range keys {
		if v, ok := from[key]; ok {
			// don't override a non-zero length value with zero length
			if len(v) > 0 || len(to[key]) == 0 {
				to[key] = []byte(v)
			}
		}
	}
	return to
}

// parseExternalSecretDataRef knows how to parse the remote data ref string, returns the
// corresponding ExternalSecretRef object.
func parseExternalSecretDataRef(dataRefStr string) (*apiv1.ExternalSecretRef, error) {
	uri, err := url.Parse(dataRefStr)
	if err != nil {
		return nil, err
	}

	ref := &apiv1.ExternalSecretRef{}
	if len(uri.Path) > 0 {
		partialName, property := parsePath(uri.Path)
		if len(partialName) > 0 {
			ref.Name = uri.Host + "/" + partialName
		} else {
			ref.Name = uri.Host
		}
		ref.Property = property
	} else {
		ref.Name = uri.Host
	}

	query := uri.Query()
	if len(query) > 0 && len(query.Get("version")) > 0 {
		ref.Version = query.Get("version")
	}

	return ref, nil
}

func parsePath(path string) (partialName string, property string) {
	pathParts := strings.Split(path, "/")
	if len(pathParts) > 1 {
		partialName = strings.Join(pathParts[1:len(pathParts)-1], "/")
		property = pathParts[len(pathParts)-1]
	} else {
		property = pathParts[0]
	}
	return partialName, property
}

func initBasicSecret(namespace, name string, secretType v1.SecretType, immutable bool) *v1.Secret {
	secret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Immutable: &immutable,
		Type:      secretType,
	}
	return secret
}
