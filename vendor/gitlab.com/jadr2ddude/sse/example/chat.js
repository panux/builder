var bod = document.getElementById('body');
var username;

function doLogin() {
    username = document.getElementById('username').value;
    bod.innerHTML = '<div id="log"><p>Logged in</p></div><br><input id="text"><button id="send" type="button" onclick="doSend()">Send</button>';
    var sse = new EventSource('/event');
    sse.onerror = function() {
        alert("SSE failed!");
    };
    sse.onopen = function() {
        console.log('SSE started.');
    };
    sse.onmessage = function(e) {
        var m = JSON.parse(e.data);
        var p = document.createElement('p');
        var uname = document.createElement('b');
        uname.appendChild(document.createTextNode(m.user));
        p.appendChild(uname);
        p.appendChild(document.createTextNode(' '+m.text));
        document.getElementById('log').appendChild(p);
    };
}

function doSend() {
    document.getElementById('send').disabled = true;
    var formData = new FormData();
    formData.append('user', username);
    formData.append('text', document.getElementById('text').value);
    var xhr = new XMLHttpRequest();
    xhr.open('POST', '/send', true);
    xhr.addEventListener('load', function(event) {
        document.getElementById('text').value = '';
        document.getElementById('send').disabled = false;
    })
    xhr.addEventListener('error', function(event) {
        document.getElementById('send').outerHTML = '<b>ERROR</b>'
    })
    xhr.send(formData);
}
