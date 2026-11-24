pipeline {
    agent none

    triggers {
        cron('0 3 * * *\n0 4 * * *')
    }

    options {
        disableConcurrentBuilds()
        timeout(time: 45, unit: 'MINUTES')
    }

    environment {
        PROJECT_NAME = 'redxt'
        PROJECT_ORG = 'webappsgo'
        CGO_ENABLED = '0'
        BUILD_IMAGE = 'casjaysdev/go:latest'
        GO_CACHE = "${JENKINS_HOME}/cache/webappsgo/redxt/go-mod"
        GO_BUILD = "${JENKINS_HOME}/cache/webappsgo/redxt/go-build"
    }

    stages {
        stage('Setup') {
            agent { docker { image 'casjaysdev/go:latest' } }
            steps {
                script {
                    if (env.TAG_NAME) {
                        env.BUILD_TYPE = 'release'
                        env.VERSION = env.TAG_NAME.replaceFirst('^v', '')
                    } else if (env.BRANCH_NAME == 'beta') {
                        env.BUILD_TYPE = 'beta'
                        env.VERSION = sh(script: 'date -u +%Y%m%d%H%M%S', returnStdout: true).trim() + '-beta'
                    } else if (env.BRANCH_NAME == 'main' || env.BRANCH_NAME == 'master') {
                        env.BUILD_TYPE = 'devel'
                        env.VERSION = sh(script: 'git rev-parse --short=7 HEAD', returnStdout: true).trim()
                    } else {
                        env.BUILD_TYPE = 'dev'
                        env.VERSION = sh(script: 'git rev-parse --short=7 HEAD', returnStdout: true).trim()
                    }
                    env.COMMIT_ID = sh(script: 'git rev-parse --short=7 HEAD', returnStdout: true).trim()
                    env.BUILD_EPOCH = sh(script: 'date -u +%s', returnStdout: true).trim()
                    env.BUILD_DATE = sh(script: "date -u -d @${env.BUILD_EPOCH} +\"%Y-%m-%dT%H:%M:%SZ\"", returnStdout: true).trim()
                    env.OFFICIAL_SITE = sh(script: '[ -f site.txt ] && cat site.txt || true', returnStdout: true).trim()
                    env.LDFLAGS = "-s -w -X 'main.Version=${env.VERSION}' -X 'main.CommitID=${env.COMMIT_ID}' -X 'main.BuildEpoch=${env.BUILD_EPOCH}' -X 'main.OfficialSite=${env.OFFICIAL_SITE}'"
                }
            }
        }
        stage('Quality') {
            agent { docker { image 'casjaysdev/go:latest' } }
            steps {
                sh 'go vet ./...'
                sh 'staticcheck ./...'
                sh '''
                    mkdir -p /tmp/webappsgo
                    COVDIR=$(mktemp -d /tmp/webappsgo/redxt-XXXXXX)
                    go test -coverprofile="$COVDIR/coverage.out" ./...
                    go tool cover -func="$COVDIR/coverage.out" | awk '/^total:/ {gsub("%", "", $3); if ($3 < 60) exit 1}'
                '''
            }
        }
        stage('Build') {
            parallel {
                stage('Linux amd64') {
                    agent { docker { image 'casjaysdev/go:latest' } }
                    steps { sh '''
                        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-linux-amd64 ./src
                        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-cli-linux-amd64 ./src/client
                        CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-agent-linux-amd64 ./src/agent
                    ''' }
                }
                stage('Linux arm64') {
                    agent { docker { image 'casjaysdev/go:latest' } }
                    steps { sh '''
                        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-linux-arm64 ./src
                        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-cli-linux-arm64 ./src/client
                        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-agent-linux-arm64 ./src/agent
                    ''' }
                }
                stage('Darwin amd64') {
                    agent { docker { image 'casjaysdev/go:latest' } }
                    steps { sh '''
                        CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-darwin-amd64 ./src
                        CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-cli-darwin-amd64 ./src/client
                        CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-agent-darwin-amd64 ./src/agent
                    ''' }
                }
                stage('Darwin arm64') {
                    agent { docker { image 'casjaysdev/go:latest' } }
                    steps { sh '''
                        CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-darwin-arm64 ./src
                        CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-cli-darwin-arm64 ./src/client
                        CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-agent-darwin-arm64 ./src/agent
                    ''' }
                }
                stage('Windows amd64') {
                    agent { docker { image 'casjaysdev/go:latest' } }
                    steps { sh '''
                        CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-windows-amd64.exe ./src
                        CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-cli-windows-amd64.exe ./src/client
                        CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-agent-windows-amd64.exe ./src/agent
                    ''' }
                }
                stage('Windows arm64') {
                    agent { docker { image 'casjaysdev/go:latest' } }
                    steps { sh '''
                        CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-windows-arm64.exe ./src
                        CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-cli-windows-arm64.exe ./src/client
                        CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-agent-windows-arm64.exe ./src/agent
                    ''' }
                }
                stage('FreeBSD amd64') {
                    agent { docker { image 'casjaysdev/go:latest' } }
                    steps { sh '''
                        CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-freebsd-amd64 ./src
                        CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-cli-freebsd-amd64 ./src/client
                        CGO_ENABLED=0 GOOS=freebsd GOARCH=amd64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-agent-freebsd-amd64 ./src/agent
                    ''' }
                }
                stage('FreeBSD arm64') {
                    agent { docker { image 'casjaysdev/go:latest' } }
                    steps { sh '''
                        CGO_ENABLED=0 GOOS=freebsd GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-freebsd-arm64 ./src
                        CGO_ENABLED=0 GOOS=freebsd GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-cli-freebsd-arm64 ./src/client
                        CGO_ENABLED=0 GOOS=freebsd GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o redxt-agent-freebsd-arm64 ./src/agent
                    ''' }
                }
            }
        }
        stage('Security') {
            parallel {
                stage('Secrets') {
                    agent { docker { image 'trufflesecurity/trufflehog:3.100.1@sha256:9c7a8325d43a4af2960d3f21d61a08981d8a1bf0542e007efa2e47097e386452' } }
                    steps {
                        sh 'trufflehog git file://. --since-commit HEAD~1 --results=verified,unknown --exclude-paths=.github/trufflehog-exclude.txt --fail'
                        sh 'trufflehog git file://. --results=verified --exclude-paths=.github/trufflehog-exclude.txt --fail'
                    }
                }
                stage('Vulnerabilities') {
                    agent { docker { image 'casjaysdev/go:latest' } }
                    steps { sh 'test ! -f go.sum || govulncheck ./...' }
                }
                stage('Workflow policy') {
                    agent { docker { image 'casjaysdev/go:latest' } }
                    steps { sh 'grep -R -nE "pull_request_target|uses:.*@[[:xdigit:]]{1,39}([^[:xdigit:]]|$)" .github/workflows .gitea/workflows .forgejo/workflows && exit 1 || true' }
                }
                stage('Image scan') {
                    agent { label 'amd64' }
                    steps {
                        sh 'docker build -f docker/Dockerfile -t redxt:ci .'
                        sh 'docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v "$PWD:/work" -w /work aquasec/trivy:0.70.0@sha256:be1190afcb28352bfddc4ddeb71470835d16462af68d310f9f4bca710961a41e image --exit-code 1 --severity HIGH,CRITICAL redxt:ci'
                    }
                }
            }
        }
        stage('Release') {
            when { tag 'v*' }
            agent { docker { image 'casjaysdev/go:latest' } }
            steps {
                sh 'cyclonedx-gomod mod -json -output redxt-sbom.cdx.json'
                sh 'echo "$VERSION" > version.txt'
                sh '''tar --exclude='.git' --exclude='.github' --exclude='.gitea' --exclude='.forgejo' --exclude='redxt-*' --exclude='version.txt' --exclude='SHA256SUMS' --exclude='SHA512SUMS' --exclude='*.tar.gz' -czf "redxt-$VERSION-source.tar.gz" .'''
                sh 'sha256sum version.txt redxt-* > SHA256SUMS'
                sh 'sha512sum version.txt redxt-* > SHA512SUMS'
                archiveArtifacts artifacts: 'redxt-*,version.txt,SHA256SUMS,SHA512SUMS', fingerprint: true
            }
        }
        stage('Release channel artifacts') {
            when { anyOf { branch 'beta'; branch 'main'; branch 'master' } }
            agent { docker { image 'casjaysdev/go:latest' } }
            steps {
                sh 'cyclonedx-gomod mod -json -output redxt-sbom.cdx.json'
                sh 'echo "$VERSION" > version.txt'
                sh '''tar --exclude='.git' --exclude='.github' --exclude='.gitea' --exclude='.forgejo' --exclude='redxt-*' --exclude='version.txt' --exclude='SHA256SUMS' --exclude='SHA512SUMS' --exclude='*.tar.gz' -czf "redxt-$VERSION-source.tar.gz" .'''
                sh 'sha256sum version.txt redxt-* > SHA256SUMS'
                sh 'sha512sum version.txt redxt-* > SHA512SUMS'
                archiveArtifacts artifacts: 'redxt-*,version.txt,SHA256SUMS,SHA512SUMS', fingerprint: true
            }
        }
        stage('Docker publish') {
            when { not { changeRequest() } }
            agent { label 'amd64' }
            steps {
                withCredentials([string(credentialsId: 'github-token', variable: 'REGISTRY_TOKEN')]) {
                    sh '''
                        echo "$REGISTRY_TOKEN" | docker login ghcr.io -u webappsgo --password-stdin
                        docker buildx create --name redxt-builder --use 2>/dev/null || docker buildx use redxt-builder
                        docker buildx build -f docker/Dockerfile --platform linux/amd64,linux/arm64 \\
                            --build-arg VERSION="$VERSION" --build-arg BUILD_DATE="$BUILD_DATE" \\
                            --build-arg BUILD_EPOCH="$BUILD_EPOCH" --build-arg COMMIT_ID="$COMMIT_ID" \\
                            -t "ghcr.io/webappsgo/redxt:$COMMIT_ID" --push .
                    '''
                }
            }
        }
        stage('Docker devel') {
            when { expression { env.BUILD_TYPE == 'devel' } }
            agent { label 'amd64' }
            steps {
                withCredentials([string(credentialsId: 'github-token', variable: 'REGISTRY_TOKEN')]) {
                    sh '''
                        echo "$REGISTRY_TOKEN" | docker login ghcr.io -u webappsgo --password-stdin
                        docker buildx create --name redxt-builder --use 2>/dev/null || docker buildx use redxt-builder
                        docker buildx build -f docker/Dockerfile.dev --platform linux/amd64,linux/arm64 \\
                            --build-arg VERSION="$VERSION" --build-arg BUILD_DATE="$BUILD_DATE" \\
                            --build-arg BUILD_EPOCH="$BUILD_EPOCH" --build-arg COMMIT_ID="$COMMIT_ID" \\
                            -t ghcr.io/webappsgo/redxt:devel --push .
                    '''
                }
            }
        }
    }
}
