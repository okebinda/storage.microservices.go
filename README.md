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

### Configure

The service uses `.env` files to configure custom values in the `serverless.yml` configuration file. It is recommended to create `.env` files for each environment (dev, stage, prod, etc.) using a template similar to the following (make sure to change the values to reflect your situation):

```
DOMAIN=domain.com
PREFIX=aws-com-domain
REGION=us-east-1
```

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

#### Upload URL

Use the following to perform a local smoke test for the Upload URL function:

```ssh
$ cd /vagrant/services/image-upload
$ sls invoke local --function image-upload  --data '{"httpMethod":"GET", "path":"image/upload-url", "queryStringParameters": {"extension":"png", "directory":"test"}}'
```

### Use

#### 1) Generate a Pre-Signed S3 Upload URL

To generate a pre-signed S3 upload URL, make a request to the public URL of the lambda function with the `directory` and `extension` parameters, for example:

```ssh
$ curl "https://XXXXXX.execute-api.us-east-1.amazonaws.com/dev/image/upload-url?directory=test&extension=png"
```

(Note that the raw output from curl has the '&' character encoded as '\u0026', which browsers and most tools will interpret correctly.)

#### 2) Upload an Image to Upload S3 Bucket

Use the `upload_url` property in the previous JSON response to upload an image using the REST PUT operation, for example:

```ssh
$ curl -X PUT -T "/vagrant/data/images/profile.png" -H "Content-Type: image/png" "https://s3.amazonaws.com/images.upload.dev.domain.com/test/90546589-e63c-4de1-bd49-042ecd20daf1.png?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=ASIA3SOD5AGO45XV5FJ3%2F20201225%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20201225T003850Z&X-Amz-Expires=900&X-Amz-Security-Token=IQoJb3JpZ2luX2VjEDkaCXVzLWVhc3QtMSJIMEYCIQCoNfgMh6SyiFQeHN2Rm%2FUfKqEcsdxh1iHXrS5wmHnImQIhAOexERwkaHhC1QdFSpBlTcDnj6ltSFjXCNuPcIvPKb9WKvABCOL%2F%2F%2F%2F%2F%2F%2F%2F%2F%2FwEQARoMNzk1NTE2NjY2MjY5IgzCza0X8F8hULnLvWQqxAG8LRiuorg2F%2FdIJh9zmjN3LpMzsuux4H2vIYomzZjCZTIG0Lib%2BDS2IGyoqEd45N7OSgxRtishQO6UC0fK6EA8G17un%2F35c0P8fX4FKRJyiTofEKkSg5pslvpn66FEpDJu8d86eObLXQDRH9DOoHuTXz5TTo%2B36kfoInj7ctsd8doh4oeFFvuS5i83o2M2JH5XTW%2FNJUTkdaS321nniWdHWd%2FFmqtuppMAIZ0wARrsixVeP2KAaaP2oJfcKxvHV70n7LLuMJrplP8FOt8BgmPE4hTij15jEswysv3XF0FEOFMA63L5eARpoD9w7VWm3RdLm4KlbAC37%2FublYu7V7jGVSdEyCazS1arAF%2BrGprOco%2F7t5pChP6qyhiy4ugAIqcPkrdgk9YAU8VXwIQbwYi9WDAiThnV1TF7FynIVd3uhOIFOzypUAbiUGgvUZH17VLNI9L5R1xvUEC4aAglpK4sfaXq0GJpryUJlnKNJHkIuVyhBX97HEP0il2o3bEX1ab6ig1EQFWp66PMiz1Go6Yu3mOx0nKovkueed4NzlwxToeaaAR7GjU1AS%2B4Mg%3D%3D&X-Amz-SignedHeaders=content-type%3Bhost&X-Amz-Signature=d0fad517f351a8abf78634433d5a6e0b8a653bf33455c03d94fb5279b6c74a94"
```

#### 3) Process and Move the Image to the Static S3 Bucket

To complete the image upload process, POST a JSON message to the upload process function using its public URL with the following properties:

* file_id (required)
* file_extension (required)
* directory (optional)
* width (optional)
* height (optional)

For example:

```ssh
$ curl -X POST -H "Content-Type: application/json" -d '{"file_id": "90546589-e63c-4de1-bd49-042ecd20daf1", "file_extension": "png", "directory": "test", "width": 250, "height": 250}' "https://XXXXXX.execute-api.us-east-1.amazonaws.com/dev/image/process-upload"
```

#### Delete an Image

To delete an image from the static S3 bucket make a DELETE request to the public URL of the delete Lambda function with the image's key appended to the end of the URL, for example:

```ssh
$ curl -X DELETE "https://XXXXXX.execute-api.us-east-1.amazonaws.com/dev/image/delete/test/90546589-e63c-4de1-bd49-042ecd20daf1.png"
```

### Deployment

Deploy to the development environment:

```ssh
$ cd /vagrant/services/image-upload
$ sls deploy --stage dev
```

Deploy to the production environment:

```ssh
$ cd /vagrant/services/image-upload
$ sls deploy --stage prod
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

## Service: Image Serve

The service uses `.env` files to configure custom values in the `serverless.yml` configuration file. It is recommended to create `.env` files for each environment (dev, stage, prod, etc.) using a template similar to the following (make sure to change the values to reflect your situation):

```
DOMAIN=domain.com
PREFIX=aws-com-domain
REGION=us-east-1
IMAGE_SERVE_HOSTNAME=XXXXXX.execute-api.us-east-1.amazonaws.com
```

### Install Dependencies

```ssh
$ cd /vagrant/services/image-serve
$ ./scripts/build.sh
```

### Compile

```ssh
$ cd /vagrant/services/image-serve
$ make
```

### Local Invocation

#### Resize Ratio

Use the following to perform a local smoke test for the Resize Ration function, should return a 404 error:

```ssh
$ cd /vagrant/services/image-serve
$ sls invoke local --function image-serve --data '{"httpMethod":"GET", "path":"/ratio/400x300/test/90546589-e63c-4de1-bd49-042ecd20daf1.png"}'
```

### Use

In a web browser, go to the public/website URL for the S3 image cache bucket's object, for example:

URL: http://images.cache.dev.domain.com.s3-website-us-east-1.amazonaws.com/ratio/400x300/test/90546589-e63c-4de1-bd49-042ecd20daf1.png

For custom domains and SSL certs you may want to use CloudFront to serve the S3 content.

### Deployment

Deploy to the development environment:

```ssh
$ cd /vagrant/services/image-serve
$ sls deploy --stage dev
```

Deploy to the production environment:

```ssh
$ cd /vagrant/services/image-serve
$ sls deploy --stage prod
```

### Linters

List of linters supplied with project:

* gofmt (https://golang.org/cmd/gofmt/)
* go vet (https://golang.org/cmd/vet/)
* golint (https://github.com/golang/lint)
* gosec (https://github.com/securego/gosec)

```ssh
$ cd /vagrant/service/image-serve
$ ./scripts/lint.sh
```

## Repository Directory Structure

| Directory/File                | Purpose                                                                            |
| ----------------------------- | ---------------------------------------------------------------------------------- |
| `services/`                   | Contains all source code files required for the services                           |
| `├─image-serve/`              | Contains the source code for the Image Serve service                               |
| `│· ├─bin/`                   | Contains compiled service binaries                                                 |
| `│· ├─scripts/`               | Contains scripts to build the service, run linters, and any other useful tools     |
| `|· ├─src/`                   | Contains source code for all of the Image Serve microservices                      |
| `│· ├─static/`                | Contains HTML files for the index and error pages used for S3 website hosting      |
| `│· ├─go.mod`                 | Dependency requirements                                                            |
| `│· ├─Makefile`               | Instructions for `make` to build service binaries                                  |
| `│· └─serverless.yml`         | Serverless framework configuration file                                            |
| `└─image-upload/`             | Contains the source code for the Image Upload service                              |
| ` · ├─bin/`                   | Contains compiled service binaries                                                 |
| ` · ├─scripts/`               | Contains scripts to build the service, run linters, and any other useful tools     |
| ` · ├─src/`                   | Contains source code for all of the Image Upload microservices                     |
| ` · ├─go.mod`                 | Dependency requirements                                                            |
| ` · ├─Makefile`               | Instructions for `make` to build service binaries                                  |
| ` · └─serverless.yml`         | Serverless framework configuration file                                            |
| `data/`                       | Contains additional resources, such as sample images                               |
| `documentation/`              | Documentation files                                                                |
| `provision/`                  | Provision scripts for local virtual machine and production servers                 |
| `scripts/`                    | Contains various scripts                                                           |
| `LICENSE`                     | The license that governs usage of the this source code                             |
| `README.md`                   | This file                                                                          |
| `Vagranfile`                  | Configuration file for Vagrant when provisioning local development virtual machine |
