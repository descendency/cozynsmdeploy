<html>
<head>
<title></title>
</head>
<body>
<form action="/deploy" method="post">
    Sensor Collection Interface: <select type="text" name="interface">
      {{range .Interface}}
      <option>{{.}}</option>
      {{end}}
    </select><br />
    Application Interface: <select type="text" name="appinterface">
      {{range .AppInterface}}
      <option>{{.}}</option>
      {{end}}
    </select><br />
    IP Schema:<input type="text" name="ip" value="{{.IP}}"/><br />
    Domain:<input type="text" name="domain" /><br />
    IPA Admin Password: <input type="text" name="ipapassword" /><br />
    Bro Workers:<input type="text" name="workers" /><br />
    ElasticSearch RAM:<input type="text" name="memory" /><br /> <hr/>
    <input type="submit" value="Deploy" />
</form>
</body>
</html>
