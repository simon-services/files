# files-service 

### license
BSD 1-Clause


### dependencies
- minio s3 storage (Apache-2.0)
- echo golang server (MIT)
- GNU make (GPLv3)

### usage
```
# build the files service
make build

# have a look on the arguments you can provide to start the service
./files.{OS}.amd64 start --help
```

### start with expected settings
- will start the service at 0.0.0.0:7878
- expects the webroot path to be ./webroot
- minio key and secret should be left default
- minio port 9000 is used

