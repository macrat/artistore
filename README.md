Artistore
=========

A simple artifact store server.


## Usage

### 1. Generate server secret

``` shell
$ export ARTISTORE_SECRET=$(artistore secret)
```

### 2. Start server

``` shell
$ artistore serve --store ./path/to/storage
```

### 3. Publish your artifact

``` shell
$ artistore publish prefix/filename.txt
```

or

``` shell
$ export ARTISTORE_TOKEN=$(artistore token prefix/)
$ curl -H "Authorization: bearer ${ARTISTORE_TOKEN}" --data-binary '@file.txt' http://localhost:3000/prefix/filename.txt
```

### 4. Download and use your artifact

``` shell
$ artistore get prefix/filename.txt
```

or

``` shell
$ curl -L https://localhost:3000/prefix/filename.txt
```
