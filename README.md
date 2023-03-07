# clank

clank is a simple tool that allows you to detect imposter commits in GitHub
Actions workflows.

This is primarily a proof-of-concept - our aim is to upstream this check to
[OpenSSF Scorecards](https://github.com/ossf/scorecard).

The name is inspired by https://github.com/sethvargo/ratchet.

## Installation

```sh
$ go install github.com/chainguard-dev/clank@latest
```

## Usage

```sh
$ clank [ path/to/workflow/dir | URL ]
```

### Examples:

By path:

```sh
$ clank ./testdata
testdata/push.yaml
+---------------------------------------------------------------------+--------+-------+-------------------------+
|                                 REF                                 | STATUS | LINES |         DETAILS         |
+---------------------------------------------------------------------+--------+-------+-------------------------+
| actions://actions/checkout@main                                     | OK     | [10]  |                         |
| actions://actions/checkout@c7d749a2d57b4b375d1ebcd17cfbfb60c676f18e | ERROR  | [7]   | SHA not present in repo |
+---------------------------------------------------------------------+--------+-------+-------------------------+
```

By URL:

```sh
$ clank https://github.com/sigstore/cosign
/var/folders/83/j7crs0zj5g9_nj3wb9hql9hh0000gn/T/clank-3841068745/sigstore/cosign/.github/workflows/build.yaml
+-------------------------------------------------------------------------------+--------+-------+---------+
|                                      REF                                      | STATUS | LINES | DETAILS |
+-------------------------------------------------------------------------------+--------+-------+---------+
| actions://sigstore/cosign-installer@c3667d99424e7e6047999fb6246c0da843953c65  | OK     | [46]  |         |
| actions://actions/setup-go@6edd4406fa81c3da01a34fa6f6343087c207a568           | OK     | [48]  |         |
| actions://ko-build/setup-ko@ace48d793556083a76f1e3e6068850c1f4a369aa          | OK     | [54]  |         |
| actions://google-github-actions/auth@ef5d53e30bbcd8d0836f4288f5e50ff3e086997d | OK     | [57]  |         |
| actions://actions/checkout@ac593985615ec2ede58e132d2e21d2b1cbd6127c           | OK     | [44]  |         |
+-------------------------------------------------------------------------------+--------+-------+---------+

/var/folders/83/j7crs0zj5g9_nj3wb9hql9hh0000gn/T/clank-3841068745/sigstore/cosign/.github/workflows/codeql-analysis.yml
+---------------------------------------------------------------------------------+--------+-------+---------+
|                                       REF                                       | STATUS | LINES | DETAILS |
+---------------------------------------------------------------------------------+--------+-------+---------+
| actions://actions/setup-go@6edd4406fa81c3da01a34fa6f6343087c207a568             | OK     | [63]  |         |
| actions://github/codeql-action/init@32dc499307d133bb5085bae78498c0ac2cf762d5    | OK     | [70]  |         |
| actions://github/codeql-action/analyze@32dc499307d133bb5085bae78498c0ac2cf762d5 | OK     | [78]  |         |
| actions://actions/checkout@ac593985615ec2ede58e132d2e21d2b1cbd6127c             | OK     | [50]  |         |
| actions://actions/cache@69d9d449aced6a2ede0bc19182fadc3a0a42d2b0                | OK     | [53]  |         |
+---------------------------------------------------------------------------------+--------+-------+---------+

...
```

## Authentication

clank looks for an access token to be passed in via the `GITHUB_TOKEN`
environment variable. This token is used to fetch content and compute diffs.

While clank can be used against public repos without a token, you may run into
rate limiting without it.

The easiest way to get a token is to run:

```sh
$ export GITHUB_TOKEN=`gh auth token`
$ clank ./testdata
```
