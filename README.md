# NFS Prober
Golang NFS prober to measure mounting and read/write performance of multiple NFS instances. (Linux Only)

## Prepare NFS instances

Create a folder called 'prober' in the root level of each NFS target as this application can only mount to the directory called prober, for safety reasons. This can be done like shown below.
```sh
$ mount 192.168.1.2:/nfs1 mymount && mkdir mymount/prober
```

## Running
### Flags

| Flag                 | Default       | Description  |
| -------------------- |-------------|-----------|
| --targets        | ""                  |    comma seperated list of targets in format ip:/mountPoint,ip:/mountPoint  |
| --use_prometheus       | true                   | create a web endpoint and log timeseries metrics to that endpoint   |
| --local_mount_dir      | "/etc/prober-nfs"      |   local directory to mount NFS targets in  |
| --rw_test_files        | false                  |    read and write test files after mounting at each probe interation  |
| --num_of_files         | 1                      |    number of test files to read and write to each NFS target  |
| --file_size_bytes        | 200                  |    test file size in bytes |
| --interval        | "60s"                  |    interval between each probe interation, valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h"  |
| --timeout        | false                  |    timeout of probe operation, valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h"  |
| --port        | 8080                  |    port for the web server to listen on  |



### Using Go
```bash
/home/ddlfcloud/nfs-prober# go run main.go --targets 192.168.1.2:/nfs0,192.168.1.3:/nfs1 --rw_test_files --local_mount_dir /home/ddlfcloud/nfs-prober/mymount
INFO[0000] starting HTTP endpoint on :8080              
INFO[0068] mount successful                              address=192.168.1.2 duration=0.006362586 mountPoint=/nfs0/prober success=true
INFO[0068] write test file                               address=192.168.1.2 duration=0.053528649 file=/home/ddlfcloud/nfs-prober/mymount/192.168.1.2/0 mountPoint=/nfs0/prober success=true
INFO[0068] read test file                                address=192.168.1.2 duration=0.000411045 file=/home/ddlfcloud/nfs-prober/mymount/192.168.1.2/0 mountPoint=/nfs0/prober success=true
INFO[0090] mount successful                              address=192.168.1.3 duration=0.006661706 mountPoint=/nfs1/prober success=true
INFO[0090] write test file                               address=192.168.1.3 duration=0.008783817 file=/home/ddlfcloud/nfs-prober/mymount/192.168.1.3/0 mountPoint=/nfs1/prober success=true
INFO[0090] read test file                                address=192.168.1.3 duration=0.000383989 file=/home/ddlfcloud/nfs-prober/mymount/192.168.1.3/0 mountPoint=/nfs1/prober success=true
```

### Using Docker
```bash
docker build -t nfs-prober .
docker run --privileged=true -p 8080:8080 nfs-prober --targets 192.168.1.2:/nfs0,192.168.1.3:/nfs1 --rw_test_files
```

### Metrics
Metrics are served in the log stdout or at: http://localhost:8080/metrics using prometheus data types https://prometheus.io/docs/concepts/data_model/

## FAQ

-  Q: Could this potentially overwrite my files ?
-  A: No, unless you have any files in a folder called prober eg: '192.168.1.2:/nfs0/prober'. This application mounts the   NFS targets prober folder so the code can't access anything outside that. If the folder doesn't exist the probe fails, please see setup section above. As always please use your own testing and judgement before running in production.

-  Q: Can I use an interval of 5s or less ?
-  A: Yes, but you're simply wasting resources by doing this. Use a reasonable interval like 60s or more. 

-  Q: I'm getting the error err="operation not permitted", I have to use sudo or docker --privileged=true flag, this seems dangerous.
-  A: Yes it can be, but you have to mount the NFS instance and you need root privileges to do that. Don't run this prober on a machine that has a public IP.

-  Q: I don't like this or there's something wrong.
-  A: Submit an issue or a PR or simply fork the repo.








