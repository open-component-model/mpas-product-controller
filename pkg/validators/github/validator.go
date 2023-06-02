package github

import (
	"context"
	"fmt"

	ggithub "github.com/google/go-github/v52/github"
	"golang.org/x/oauth2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	deliveryv1alpha1 "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	gitv1alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"

	"github.com/open-component-model/mpas-product-controller/pkg/validators"
)

var (
	statusCheckName = "mpas/validation-check"
	provider        = "github"
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
		logger.Info("github validator doesn't validate provider, skipping to next", "provider", repository.Spec.Provider)

		if v.Next == nil {
			return fmt.Errorf("%s not supported, and no next validator configured", repository.Spec.Provider)
		}

		return v.Next.FailValidation(ctx, repository, sync)
	}

	return v.createCheckRunStatus(ctx, repository, sync.Status.PullRequestID, "success")
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

	return v.createCheckRunStatus(ctx, repository, sync.Status.PullRequestID, "error")
}

func (v *Validator) createCheckRunStatus(ctx context.Context, repository gitv1alpha1.Repository, pullRequestID int, status string) error {
	token, err := v.retrieveAccessToken(ctx, repository)
	if err != nil {
		return fmt.Errorf("failed to retrieve token: %w", err)
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: string(token)})
	tc := oauth2.NewClient(context.Background(), ts)

	g := ggithub.NewClient(tc)

	pr, _, err := g.PullRequests.Get(ctx, repository.Spec.Owner, repository.Name, pullRequestID)
	if err != nil {
		return fmt.Errorf("failed to find PR: %w", err)
	}

	_, _, err = g.Repositories.CreateStatus(ctx, repository.Spec.Owner, repository.Name, *pr.Head.SHA, &ggithub.RepoStatus{
		// State is the current state of the repository. Possible values are:
		// pending, success, error, or failure.
		State:       &status,
		Description: ggithub.String("MPAS Validation Check"),
		Context:     ggithub.String(statusCheckName),
	})

	if err != nil {
		return fmt.Errorf("failed to create status for pr: %w", err)
	}

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

	token, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("password not found in secret")
	}

	return token, nil
}
