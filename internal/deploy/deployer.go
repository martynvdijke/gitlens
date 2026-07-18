package deploy

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
)

// Deployer pulls a Docker image and updates the target container.
type Deployer interface {
	PullAndUpdate(ctx context.Context, target Target, tag string) error
}

// NewDeployer creates a Deployer based on DEPLOY_BACKEND env var.
//   - "api" (default): docker pull + stop + rm + create + start
//   - "compose": docker compose pull + up -d --no-deps <service>
func NewDeployer() Deployer {
	switch DeployBackend() {
	case "compose":
		return &composeDeployer{}
	default:
		return &dockerDeployer{
			inflight: make(map[string]*containerLock),
		}
	}
}

// containerLock provides per-container serialization.
type containerLock struct {
	ch chan struct{}
	mu sync.Mutex
}

func newContainerLock() *containerLock {
	return &containerLock{ch: make(chan struct{}, 1)}
}

func (l *containerLock) Lock()    { l.ch <- struct{}{} }
func (l *containerLock) Unlock()  { <-l.ch }

type dockerDeployer struct {
	mu       sync.Mutex
	inflight map[string]*containerLock
}

func (d *dockerDeployer) getLock(container string) *containerLock {
	d.mu.Lock()
	defer d.mu.Unlock()
	lk, ok := d.inflight[container]
	if !ok {
		lk = newContainerLock()
		d.inflight[container] = lk
	}
	return lk
}

func (d *dockerDeployer) PullAndUpdate(ctx context.Context, target Target, tag string) error {
	lk := d.getLock(target.Container)
	lk.Lock()
	defer lk.Unlock()

	imageRef := target.Image + ":" + tag
	log.Printf("Deploy: pulling image %s", imageRef)

	if err := execCmd(ctx, "docker", "pull", imageRef); err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	// Check if container exists
	exists := execCmd(ctx, "docker", "inspect", target.Container) == nil

	if !exists {
		log.Printf("Deploy: container %s does not exist, creating...", target.Container)
		if err := execCmd(ctx, "docker", "create", "--name", target.Container, imageRef); err != nil {
			return fmt.Errorf("create failed: %w", err)
		}
		if err := execCmd(ctx, "docker", "start", target.Container); err != nil {
			return fmt.Errorf("start failed: %w", err)
		}
		log.Printf("Deploy: container %s created with %s", target.Container, imageRef)
		return nil
	}

	log.Printf("Deploy: stopping container %s", target.Container)
	if err := execCmd(ctx, "docker", "stop", target.Container); err != nil {
		return fmt.Errorf("stop failed: %w", err)
	}

	log.Printf("Deploy: removing container %s", target.Container)
	if err := execCmd(ctx, "docker", "rm", target.Container); err != nil {
		return fmt.Errorf("rm failed: %w", err)
	}

	log.Printf("Deploy: creating container %s with %s", target.Container, imageRef)
	if err := execCmd(ctx, "docker", "create", "--name", target.Container, imageRef); err != nil {
		return fmt.Errorf("create failed: %w", err)
	}

	if err := execCmd(ctx, "docker", "start", target.Container); err != nil {
		return fmt.Errorf("start failed: %w", err)
	}

	log.Printf("Deploy: container %s updated to %s", target.Container, imageRef)
	return nil
}

// composeDeployer runs docker compose pull + up -d for the service.
type composeDeployer struct{}

func (d *composeDeployer) PullAndUpdate(ctx context.Context, target Target, tag string) error {
	log.Printf("Deploy (compose): pulling service %s", target.Container)
	if err := execCmd(ctx, "docker", "compose", "pull", target.Container); err != nil {
		return fmt.Errorf("compose pull failed: %w", err)
	}

	log.Printf("Deploy (compose): recreating service %s", target.Container)
	if err := execCmd(ctx, "docker", "compose", "up", "-d", "--no-deps", target.Container); err != nil {
		return fmt.Errorf("compose up failed: %w", err)
	}

	log.Printf("Deploy (compose): service %s updated", target.Container)
	return nil
}

// execCmd runs a command and returns an error if it fails.
func execCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w\n%s", name, args, err, string(out))
	}
	return nil
}
