package check

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/concourse/concourse-pipeline-resource/concourse"
	"github.com/concourse/concourse-pipeline-resource/concourse/api"
	"github.com/concourse/concourse-pipeline-resource/fly"
	"github.com/concourse/concourse-pipeline-resource/logger"
)

type CheckCommand struct {
	logger      logger.Logger
	logFilePath string
	flyConn     fly.FlyConn
	apiClient   api.Client
}

func NewCheckCommand(
	logger logger.Logger,
	logFilePath string,
	flyConn fly.FlyConn,
	apiClient api.Client,
) *CheckCommand {
	return &CheckCommand{
		logger:      logger,
		logFilePath: logFilePath,
		flyConn:     flyConn,
		apiClient:   apiClient,
	}
}

func (c *CheckCommand) Run(input concourse.CheckRequest) (concourse.CheckResponse, error) {
	logDir := filepath.Dir(c.logFilePath)
	existingLogFiles, err := filepath.Glob(filepath.Join(logDir, "concourse-pipeline-resource-check.log*"))
	if err != nil {
		// This is untested because the only error returned by filepath.Glob is a
		// malformed glob, and this glob is hard-coded to be correct.
		return nil, err
	}

	for _, f := range existingLogFiles {
		if filepath.Base(f) != filepath.Base(c.logFilePath) {
			c.logger.Debugf("Removing existing log file: %s\n", f)
			err := os.Remove(f)
			if err != nil {
				// This is untested because it is too hard to force os.Remove to return
				// an error.
				return nil, err
			}
		}
	}

	c.logger.Debugf("Received input: %+v\n", input)

	insecure := false
	if input.Source.Insecure != "" {
		var err error
		insecure, err = strconv.ParseBool(input.Source.Insecure)
		if err != nil {
			return concourse.CheckResponse{}, err
		}
	}

	teams := make(map[string]concourse.Team)

	for _, team := range input.Source.Teams {
		teams[team.Name] = team
	}

	pipelineVersions := make(map[string]string)

	for teamName, team := range teams {
		c.logger.Debugf("Performing login\n")
		_, err := c.flyConn.Login(
			input.Source.Target,
			teamName,
			team.Username,
			team.Password,
			insecure,
		)
		if err != nil {
			return concourse.CheckResponse{}, err
		}

		c.logger.Debugf("Login successful\n")

		pipelines, err := c.apiClient.Pipelines(teamName)
		if err != nil {
			return concourse.CheckResponse{}, err
		}
		c.logger.Debugf("Found pipelines (%s): %+v\n", teamName, pipelines)

		for _, pipeline := range pipelines {
			c.logger.Debugf("Getting pipeline: %s\n", pipeline.Name)
			outBytes, err := c.flyConn.GetPipeline(pipeline.Name)
			if err != nil {
				return concourse.CheckResponse{}, err
			}

			version := fmt.Sprintf(
				"%x",
				md5.Sum(outBytes),
			)
			pipelineVersions[pipeline.Name] = version
		}
	}

	out := concourse.CheckResponse{
		pipelineVersions,
	}

	c.logger.Debugf("Returning output: %+v\n", out)

	return out, nil
}
