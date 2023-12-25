# Contributing

## Introduction

The `container-app-operator` is an application using the Operator Pattern for running inside a container in a Kubernetes cluster.

## Makefile

The `Makefile` provides multiple targets for testing, building and running `Capp`. The most common used targets are mentioned below.

## What to do before submitting a pull request

### Running unit tests

In order to verify your changes didn't break anything, you can run `make test`. It runs unit tests, but also various other pre-build tasks, in order to ensure that none of them is forgotten.

```bash
$ make test
```

### Running e2e tests

In order to run e2e tests against a real cluster, you need to make sure you have all prerequisites installed on the cluster. You can use the following `Makefile` target:

```bash
$ make prereq
```

Afterwards, run the following `Makefile` target:

```bash
$ make test-e2e
```

## Building and Running Capp

Follow the Getting Started entry in the [`README.md`](README.md)

## Versioning and Branching

`Capp` follows [Semantic Versioning](https://semver.org). 

- an X (*major*) release indicates a set of backwards-compatible code. Changing X means there's a breaking change.

- a Y (*minor*) release indicates a minimum feature set. Changing Y means the addition of a backwards-compatible feature.

- a Z (*patch*) release indicates minimum set of bugfixes. Changing Z means a backwards-compatible change that doesn't add functionality.

*NB*: If the major release is `0`, any minor release may contain breaking changes.

### Branches

`Capp` contains two types of branches: the master branch and release-X branches.

The _master_ branch is where development happens. All the latest code, including breaking changes, happens on master.

The _release-X_ branches contain stable, backwards compatible code. Every major (X) release, a new such branch is created. It is from these branches that minor and patch releases are tagged.

### PR Process

1. Submit an issue describing your proposed change to the repo in question.

2. Fork the desired repo, develop and test your code changes.

3. Submit a pull request.

#### Commands and Workflow

`Capp` follows the standard Kubernetes workflow: any PR needs `lgtm` and `approved` labels. See the [OWNERS](OWNERS) file to see who has permission to approve PRs. PRs must also pass the tests before being merged.

See more [information here](https://github.com/kubernetes/community/blob/master/contributors/guide/pull-requests.md#the-testing-and-merge-workflow).

### Release Process

1. Generate release notes using the release note tooling.

2. Add a release for the project on GitHub, using those release notes, with a title of `vX.Y.Z`.

## Where the CI Tests are configured

See the [action files](.github/workflows) to check its tests, and the scripts used on it.

