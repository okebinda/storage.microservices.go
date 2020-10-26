# STORAGE.MICROSERVICES.GO

This repository holds the source code for a few microservices used to store and manipulate files in AWS S3 buckets, written in Go using the Serverless framework for cloud management and deployment.

Local development is run on a local virtual machine managed by Vagrant. To install the virtual machine, make sure you have installed Vagrant (https://www.vagrantup.com/) and a virtual machine provider, such as VirtualBox (https://www.virtualbox.org/).

## Manage Local Development Environment

### Provision Virtual Machine

Sets up the local development environment.

```ssh
> vagrant up
> vagrant ssh
```

#### Configure AWS CLI

In order to use Serverless with AWS, you will need to configure your AWS CLI client from inside the VM:

```ssh
$ aws configure
```

### Start Virtual Machine

Starts the local development environment and logs in to the virtual machine. This is a prerequisite for any following steps if the machine is not already booted.

```ssh
> vagrant up
> vagrant ssh
```

### Stop Virtual Machine

Stops the local development environment. Run this command from the host (i.e. log out of any virtual machine SSH sessions).

```ssh
> vagrant halt
```

### Delete Virtual Machine

Deletes the local development environment. Run this command from the host (i.e. log out of any virtual machine SSH sessions).

```ssh
> vagrant destroy
```

Sometimes it is useful to completely remove all residual Vagrant files after destroying the box, in this case run the additional command:

```ssh
> rm -rf ./vagrant
```

## Service: Image Upload

### Install Dependencies

```ssh
$ cd /vagrant/services/image-upload
$ ./scripts/build.sh
```

### Compile

```ssh
$ cd /vagrant/services/image-upload
$ make
```

### Local Invocation

```ssh
$ cd /vagrant/services/image-upload
$ sls invoke local --function upload-url --data '{"queryStringParameters": {"extension":"png", "directory":"test"}}'
```

```ssh
$ cd /vagrant/service/image-upload
$ sls invoke local --function upload-image --data '{"queryStringParameters": {}}'
```

URL: https://h5xksuk60c.execute-api.us-east-1.amazonaws.com/dev/upload-url?extension=png&directory=test

### Deployment

```ssh
$ cd /vagrant/services/image-upload
$ sls deploy
```

### Linters

List of linters supplied with project:

* gofmt (https://golang.org/cmd/gofmt/)
* go vet (https://golang.org/cmd/vet/)
* golint (https://github.com/golang/lint)
* gosec (https://github.com/securego/gosec)

```ssh
$ cd /vagrant/service/image-upload
$ ./scripts/lint.sh
```


## Repository Directory Structure

| Directory/File                | Purpose                                                                            |
| ----------------------------- | ---------------------------------------------------------------------------------- |
| `services/`                   | Contains all source code files required for the services                           |
| `└─image-upload/`             | Contains the source code for the Image Upload microservice                         |
| ` · ├─bin/`                   | Contains compiled service binaries                                                 |
| ` · ├─upload-image/`          | Contains source code for the Upload Image microservice                             |
| ` · ├─upload-image-callback/` | Contains source code for the Upload Image Callback microservice                    |
| ` · ├─upload-url/`            | Contains source code for the Upload URL microservice                               |
| ` · ├─go.mod`                 | Dependency requirements                                                            |
| ` · ├─Makefile`               | Instructions for `make` to build service binaries                                  |
| ` · └─serverless.yml`         | Serverless framework configuration file                                            |
| `documentation/`              | Documentation files                                                                |
| `provision/`                  | Provision scripts for local virtual machine and production servers                 |
| `scripts/`                    | Contains various scripts                                                           |
| `LICENSE`                     | The license that governs usage of the this source code                             |
| `README.md`                   | This file                                                                          |
| `Vagranfile`                  | Configuration file for Vagrant when provisioning local development virtual machine |
