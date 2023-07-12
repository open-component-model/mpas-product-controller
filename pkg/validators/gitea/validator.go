package gitea

import (
	"context"
	"fmt"

	"code.gitea.io/sdk/gitea"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	deliveryv1alpha1 "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	gitv1alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"

	"github.com/open-component-model/mpas-product-controller/pkg/validators"
)

const (
	statusCheckName = "mpas/validation-check"
	tokenKey        = "password"
	provider        = "gitea"
	defaultDomain   = "gitea.com"
)

type Validator struct {
	Client client.Client
	Next   validators.Validator
}

func NewValidator(client client.Client, next validators.Validator) *Validator {
	return &Validator{
		Client: client,
		Next:   next,
	}
}

func (v *Validator) FailValidation(ctx context.Context, repository gitv1alpha1.Repository, sync deliveryv1alpha1.Sync) error {
	logger := log.FromContext(ctx)
	if repository.Spec.Provider != provider {
		logger.Info("gitea validator doesn't validate provider, skipping to next", "provider", repository.Spec.Provider)

		if v.Next == nil {
			return fmt.Errorf("%s not supported, and no next validator configured", repository.Spec.Provider)
		}

		return v.Next.FailValidation(ctx, repository, sync)
	}

	return v.createCheckRunStatus(ctx, repository, sync.Status.PullRequestID, gitea.StatusError)
}

func (v *Validator) PassValidation(ctx context.Context, repository gitv1alpha1.Repository, sync deliveryv1alpha1.Sync) error {
	logger := log.FromContext(ctx)
	if repository.Spec.Provider != provider {
		logger.Info("github validator doesn't validate provider, skipping to next", "provider", repository.Spec.Provider)

		if v.Next == nil {
			return fmt.Errorf("%s not supported, and no next validator configured", repository.Spec.Provider)
		}

		return v.Next.PassValidation(ctx, repository, sync)
	}

	return v.createCheckRunStatus(ctx, repository, sync.Status.PullRequestID, gitea.StatusSuccess)
}

func (v *Validator) createCheckRunStatus(ctx context.Context, repository gitv1alpha1.Repository, pullRequestID int, status gitea.StatusState) error {
	logger := log.FromContext(ctx)

	logger.Info("updating validation status", "status", status, "pullRequestID", pullRequestID)
	token, err := v.retrieveAccessToken(ctx, repository)
	if err != nil {
		return fmt.Errorf("failed to retrieve token: %w", err)
	}

	domain := defaultDomain
	if repository.Spec.Domain != "" {
		domain = repository.Spec.Domain
	}

	client, err := gitea.NewClient(domain, gitea.SetToken(string(token)))
	if err != nil {
		return fmt.Errorf("failed to create gitea client: %w", err)
	}

	pr, _, err := client.GetPullRequest(repository.Spec.Owner, repository.Name, int64(pullRequestID))
	if err != nil {
		return fmt.Errorf("failed to find PR: %w", err)
	}

	sha := pr.Head.Sha
	logger.V(4).Info("updating SHA", "sha", sha)
	state, _, err := client.CreateStatus(repository.Spec.Owner, repository.Name, sha, gitea.CreateStatusOption{
		State: status,
		//TargetURL:   "",
		Description: "MPAS Validation Check",
		Context:     statusCheckName,
	})
	if err != nil {
		return fmt.Errorf("failed to create status for pr: %w", err)
	}

	logger.V(4).Info("status", "status", state.State, "id", state.ID, "context", state.Context)
	return nil
}

func (v *Validator) retrieveAccessToken(ctx context.Context, obj gitv1alpha1.Repository) ([]byte, error) {
	secret := &corev1.Secret{}
	if err := v.Client.Get(ctx, types.NamespacedName{
		Name:      obj.Spec.Credentials.SecretRef.Name,
		Namespace: obj.Namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	token, ok := secret.Data[tokenKey]
	if !ok {
		return nil, fmt.Errorf("password not found in secret")
	}

	return token, nil
}
