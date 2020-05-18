// Copyright 2020 The PipeCD Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubernetes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kapetaniosci/pipe/pkg/app/piped/executor"
	"github.com/kapetaniosci/pipe/pkg/app/piped/toolregistry"
	"github.com/kapetaniosci/pipe/pkg/config"
	"github.com/kapetaniosci/pipe/pkg/model"
)

const (
	variantLabel    = "pipecd.dev/variant"
	managedByLabel  = "pipecd.dev/managed-by"
	commitHashLabel = "pipecd.dev/commit-hash"

	kustomizationFileName = "kustomization.yaml"
)

type TemplatingMethod string

const (
	TemplatingMethodHelm      TemplatingMethod = "helm"
	TemplatingMethodKustomize TemplatingMethod = "kustomize"
	TemplatingMethodNone      TemplatingMethod = "none"
)

type Executor struct {
	executor.Input

	appDirPath       string
	templatingMethod TemplatingMethod
}

type registerer interface {
	Register(stage model.Stage, f executor.Factory) error
}

func Register(r registerer) {
	f := func(in executor.Input) executor.Executor {
		return &Executor{
			Input: in,
		}
	}
	r.Register(model.StageK8sPrimaryUpdate, f)
	r.Register(model.StageK8sStageRollout, f)
	r.Register(model.StageK8sStageClean, f)
	r.Register(model.StageK8sBaselineRollout, f)
	r.Register(model.StageK8sBaselineClean, f)
	r.Register(model.StageK8sTrafficSplit, f)
}

func (e *Executor) Execute(ctx context.Context) model.StageStatus {
	e.appDirPath = filepath.Join(e.RepoDir, e.Deployment.GitPath.Path)
	e.templatingMethod = determineTemplatingMethod(e.DeploymentConfig, e.appDirPath)

	e.Logger.Info("start executing kubernetes stage",
		zap.String("stage-name", e.Stage.Name),
		zap.String("app-dir-path", e.appDirPath),
		zap.String("templating-method", string(e.templatingMethod)),
	)

	switch model.Stage(e.Stage.Name) {
	case model.StageK8sPrimaryUpdate:
		return e.ensurePrimaryUpdate(ctx)
	case model.StageK8sStageRollout:
		return e.ensureStageRollout(ctx)
	case model.StageK8sStageClean:
		return e.ensureStageClean(ctx)
	case model.StageK8sBaselineRollout:
		return e.ensureBaselineRollout(ctx)
	case model.StageK8sBaselineClean:
		return e.ensureBaselineClean(ctx)
	case model.StageK8sTrafficSplit:
		return e.ensureTrafficSplit(ctx)
	}

	e.Logger.Error("unsupported stage for kubernetes application",
		zap.String("stage-name", e.Stage.Name),
	)
	return model.StageStatus_STAGE_FAILURE
}

func (e *Executor) ensurePrimaryUpdate(ctx context.Context) model.StageStatus {
	_, _ = e.findKubectl(ctx, "1.8.0")
	return model.StageStatus_STAGE_SUCCESS
}

func (e *Executor) ensureStageRollout(ctx context.Context) model.StageStatus {
	return model.StageStatus_STAGE_SUCCESS
}

func (e *Executor) ensureStageClean(ctx context.Context) model.StageStatus {
	return model.StageStatus_STAGE_SUCCESS
}

func (e *Executor) ensureBaselineRollout(ctx context.Context) model.StageStatus {
	return model.StageStatus_STAGE_SUCCESS
}

func (e *Executor) ensureBaselineClean(ctx context.Context) model.StageStatus {
	return model.StageStatus_STAGE_SUCCESS
}

func (e *Executor) ensureTrafficSplit(ctx context.Context) model.StageStatus {
	return model.StageStatus_STAGE_SUCCESS
}

func (e *Executor) ensureRollback(ctx context.Context) model.StageStatus {
	return model.StageStatus_STAGE_SUCCESS
}

func (e *Executor) generateStageManifests(ctx context.Context) model.StageStatus {
	return model.StageStatus_STAGE_SUCCESS
}

func (e *Executor) generateBaselineManifests(ctx context.Context) model.StageStatus {
	return model.StageStatus_STAGE_SUCCESS
}

func (e *Executor) findKubectl(ctx context.Context, version string) (*Kubectl, error) {
	path, installed, err := toolregistry.DefaultRegistry().Kubectl(ctx, version)
	if err != nil {
		e.LogPersister.AppendError(fmt.Sprintf("Failed while installing kubectl %s (%v)", version, err))
		return nil, err
	}
	if installed {
		e.LogPersister.AppendInfo(fmt.Sprintf("Kubectl %s has just been installed because of no pre-installed binary for that version", version))
	}
	return &Kubectl{
		execPath: path,
	}, nil
}

func (e *Executor) findKustomize(ctx context.Context, version string) (*Kustomizectl, error) {
	path, installed, err := toolregistry.DefaultRegistry().Kustomize(ctx, version)
	if err != nil {
		e.LogPersister.AppendError(fmt.Sprintf("Failed while installing kustomize %s (%v)", version, err))
		return nil, err
	}
	if installed {
		e.LogPersister.AppendInfo(fmt.Sprintf("Kustomize %s has just been installed because of no pre-installed binary for that version", version))
	}
	return &Kustomizectl{
		execPath: path,
	}, nil
}

func (e *Executor) findHelm(ctx context.Context, version string) (*Helmctl, error) {
	path, installed, err := toolregistry.DefaultRegistry().Helm(ctx, version)
	if err != nil {
		e.LogPersister.AppendError(fmt.Sprintf("Failed while installing helm %s (%v)", version, err))
		return nil, err
	}
	if installed {
		e.LogPersister.AppendInfo(fmt.Sprintf("Helm %s has just been installed because of no pre-installed binary for that version", version))
	}
	return &Helmctl{
		execPath: path,
	}, nil
}

func determineTemplatingMethod(deploymentConfig *config.Config, appDirPath string) TemplatingMethod {
	if input := deploymentConfig.KubernetesDeploymentSpec.Input; input != nil {
		if input.HelmChart != nil {
			return TemplatingMethodHelm
		}
		if len(input.HelmValueFiles) > 0 {
			return TemplatingMethodHelm
		}
		if input.HelmVersion != "" {
			return TemplatingMethodHelm
		}
	}
	if _, err := os.Stat(filepath.Join(appDirPath, kustomizationFileName)); err == nil {
		return TemplatingMethodKustomize
	}
	return TemplatingMethodNone
}
