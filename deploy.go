package main

import (
  "bufio"
  "bytes"
  "fmt"
  "html/template"
  "io"
  "net/http"
  "os"
  "os/exec"
  "regexp"
  "sort"
  "strings"
  "sync"

  "github.com/pkg/sftp"
  "golang.org/x/crypto/ssh"
)

// Server Configuration Files
var AppFiles string
var SensorFiles string

// Templated Script Files
var AppTemplate string
var SensorTemplate string

func main() {
  AppFiles = "App.tar.gz"
  SensorFiles = "Sensor.tar.gz"

  AppTemplate = "AppDeploy.gtpl"
  SensorTemplate = "SensorDeploy.gtpl"

  fmt.Println("Launching Configuration Page.")

  // 3 HTTP Listeners that are part of the web application.
  http.HandleFunc("/", connect)
  http.HandleFunc("/configure", configure)
  http.HandleFunc("/deploy", deploy)

  // Open the webpage to start the GUI.
  exec.Command("open", "http://localhost:8080/").Start()

  // Listen on Port 8080 and serve the webpages. Note: this function is
  // blocking.
  http.ListenAndServe(":8080", nil)
}

// Template for SSH Server Credentials
type ServerCred struct {
  IP string               // Server IP
  User string             // Username on the server
  Password string         // Password for username@IP.
}

// CozyStack Deployment Configurations
type Configs struct {
  IP string                     // IP Schema
  Workers string                // Numbers of Bro worker
  CollectionInterface string    // Sensor Collection Interface
  Domain string                 // Internal Domain name
  IpaPassword string            // FreeIPA password
  AppInterface string           // Application Server Interface to host services
  ESRam string                  // ES Ram in gigabytes (2-31, only)
}

// Template for interface lists
type ServerConfigs struct {
  Interface []string            // Sensor Interface List
  AppInterface []string         // Application Server Interface List
  IP string                     // IP Schema (first 3 octets)
}

// SSH Server Credentials
var appCred ServerCred          // Application Server SSH Credentials
var sensorCred ServerCred       // Sensor SSH Credentials

/**
 * Copy a file from the host to a server (like SCP)
 *
 * sc: the server's SSH credentials used for connecting
 * source: the source of the file
 * destination: the destination of the file
 *
 * Similar to the shell command "scp source sc.User@sc.IP:destination"
 */
func copyFile(sc ServerCred, source string, destination string) {
  sshConfig := &ssh.ClientConfig {
    User: sc.User,
    Auth: []ssh.AuthMethod{ ssh.Password(sc.Password) },
  }
  conn, _ := ssh.Dial("tcp", sc.IP + ":22", sshConfig)
  defer conn.Close()

  size := 1 << 15
  client, _ := sftp.NewClient(conn, sftp.MaxPacket(size))
  defer client.Close()

  w, _ := client.Create(destination)
  defer w.Close()
  f, _ := os.Open(source)
  defer f.Close()

  io.Copy(w, io.Reader(f))
}

/**
 * Run a command on a remote machine over SSH. Returns the output as a string.
 *
 * sc: the server's SSH credentials used for connecting
 * command: the command to be run. (will need appropriate permissions)
 *
 * Similar to the shell command "ssh sc.User@sc.IP command"
 *
 */
func remoteCommand(sc ServerCred, command string) string {
  sshConfig := &ssh.ClientConfig {
    User: sc.User,
    Auth: []ssh.AuthMethod{ ssh.Password(sc.Password) },
  }

  conn, _ := ssh.Dial("tcp", sc.IP + ":22", sshConfig)
  defer conn.Close()

  session, _ := conn.NewSession()

  var b bytes.Buffer
  session.Stdout = &b
  session.Run(command);

  return b.String()
}

/**
 * Gets an interface list from a server to display in the GUI. Returns the
 * interface list as a string array.
 *
 * sc: the server's SSH credentials used for connecting
 *
 * Similar to the shell command "ssh sc.User@sc.IP ip -o link show"
 */
func getInterfaceList(sc ServerCred) []string {
  sInt := remoteCommand(sc, "ip -o link show")

  intList := []string {}
  re := regexp.MustCompile("^[0-9]*:\\W([a-zA-Z0-9]+):.+$")
  for _, s := range strings.Split(sInt, "\n") {
    ooo := re.FindStringSubmatch(s)
    if (len(ooo) == 2) {
      intList = append(intList, ooo[1])
    }
  }

  sort.Strings(intList)
  return intList
}

/**
 * Once the configuration information has been received (via an HTTP POST),
 * this listener will fire off two build sessions (one for each server).
 * Upon Completion of this function, the program will terminate and the servers
 * will be configured for deployment.
 *
 * By default, this event takes two arguments, an http Response Writer and the
 * http Request.
 *
 * w: Write back a response to the http request.
 * r: The http request data.
 */
func deploy(w http.ResponseWriter, r *http.Request) {
  fmt.Println(r.Method)
  if (r.Method == "POST") {
    r.ParseForm()

    fmt.Fprintf(w, "Running Installation Scripts.")

    c := Configs {
      IP: r.Form["ip"][0],
      Workers: r.Form["workers"][0],
      Domain: r.Form["domain"][0],
      CollectionInterface: r.Form["interface"][0],
      IpaPassword: r.Form["ipapassword"][0],
      ESRam: r.Form["memory"][0] + "g",
      AppInterface: r.Form["appinterface"][0],
    }

    t, _ := template.ParseFiles("./" + SensorTemplate)
    appTemplate, _ := template.ParseFiles("./" + AppTemplate)
    appFile, _ := os.Create("./AppDeploy.sh")
    f, _ := os.Create("./SensorDeploy.sh")
    wf := bufio.NewWriter(f)
    t.Execute(wf, c)
    wf.Flush()


    wf = bufio.NewWriter(appFile)
    appTemplate.Execute(wf, c)
    wf.Flush()

    go func() {
      fmt.Println("Transferring Sensor files.")
      copyFile(sensorCred, "./" + SensorFiles, "/tmp/" + SensorFiles)
      remoteCommand(sensorCred, "tar xzvf /tmp/" + SensorFiles + " -C /tmp")
      copyFile(sensorCred, "./SensorDeploy.sh", "/tmp/Sensor/SensorDeploy.sh")
      os.Remove("./SensorDeploy.sh")
      fmt.Println("Transferring Sensor files: Complete")
      fmt.Println("Sensor Server build started.")
      remoteCommand(sensorCred, "cd /tmp/Sensor; /bin/bash /tmp/Sensor/SensorDeploy.sh")
      fmt.Println("Sensor Server Deployed.")
    }()

    go func() {
      fmt.Println("Transferring Application files.")
      copyFile(appCred, "./" + AppFiles, "/tmp/" + AppFiles)
      remoteCommand(appCred, "tar xzvf /tmp/" + AppFiles + " -C /tmp")
      copyFile(appCred, "./AppDeploy.sh", "/tmp/application/AppDeploy.sh")
      os.Remove("./AppDeploy.sh")
      fmt.Println("Transferring Application Server files: Complete")
      fmt.Println("Application Server build started.")
      remoteCommand(appCred, "cd /tmp/application; /bin/bash /tmp/Application/AppDeploy.sh")
      fmt.Println("Application Server Deployed.")
    }()

  } else {
    fmt.Fprintf(w, "That isn't going to work.")
  }
}

/**
 * This event serves the first webpage in the web application. It is designed
 * to receive the user credentials to log into the server.
 *
 * By default, this event takes two arguments, an http Response Writer and the
 * http Request.
 *
 * w: Write back a response to the http request.
 * r: The http request data.
 */
func connect(w http.ResponseWriter, r *http.Request) {
  if (r.Method == "GET") {
    t, _ := template.ParseFiles("login.gtpl")
    t.Execute(w, nil)
  }
}

/**
 * After receiving the server(s) credentials, this page is designed to receive
 * an HTTP POST request where it will ask the user to fill in specific
 * configuration information about their environment.
 *
 * By default, this event takes two arguments, an http Response Writer and the
 * http Request.
 *
 * w: Write back a response to the http request.
 * r: The http request data.
 */
func configure(w http.ResponseWriter, r *http.Request) {
  if (r.Method == "POST") {
    r.ParseForm()

    // Get Sensor SSH credentials from webform
    sensorCred = ServerCred {
      IP: r.Form["ip"][0],
      User: r.Form["user"][0],
      Password: r.Form["password"][0],
    }

    // Get Application Server SSH Credentials From Webform
    appCred = ServerCred {
      IP: r.Form["appip"][0],
      User: r.Form["appuser"][0],
      Password: r.Form["apppassword"][0],
    }

    // Grab the configuration template to respond
    t, _ := template.ParseFiles("configure.gtpl")

    // The interface list on each server
    intList := getInterfaceList(sensorCred)
    aIntList := getInterfaceList(appCred)

    // Generate the IP schema from the given IPs.
    // Currently, this is limited to a /24 IP space.
    octets := strings.Split(sensorCred.IP, ".")
    var ipSchema string
    ipSchema = octets[0] + "." + octets[1] + "." + octets[2]

    // The server configuration interface that will be pushed into the configure
    // template.
    test := ServerConfigs {
      Interface: intList,
      AppInterface: aIntList,
      IP: ipSchema,
    }

    // Match the configuration with the template and then serve the template
    // to the user.
    t.Execute(w, test)
  }
}
