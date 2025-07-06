![hahaha](https://pkg.go.dev/badge/github.com/user/repo.svg)

---


<div align=center>
<img src="assets/gopm.png" width=344/>
</div>
  
# gopm

gopm is a clone of npm that can make installing packages not take 575848834 years!!

--- 

**it's def not like for production since it doesn't have vuln detecting or any of that but it's still really good**


# how to build

## on windows

  ***install the github repo so u have all the files***
  
  ### building for windows
  ```
  go build -o gopm.exe
  ```
  ### trying to build for linux on windows
  
  ```
    v:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -o gopm
   ```
  when u want to go back to windows stuff build, do:
  ```
  Remove-Item Env:GOOS
  Remove-Item Env:GOARCH
  ```
## on linux
  `go build -o gopm`
  
---

> [!IMPORTANT]
> u will need to install the golang language to build the programs (https://go.dev/dl/)
