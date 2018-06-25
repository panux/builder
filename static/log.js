// log.js is the page management code for /log.html

// pageURL is the url of the current page
var pageURL = new URL(window.location.href);

// hash is the buildHash parameter
var hash = pageURL.searchParams.get('buildHash');

function displayError(err) {
    var e = meh.elem('span');
    e.appendChild(meh.text(err));
    meh.toast(e);
}

// streamClass returns the CSS class corresponding to the line stream
function streamClass(streamnum) {
    //handle unrecognized stream number
    if(streamnum > 3) {
        streamnum = 0;
    }

    //generate stream name in the format "stream%d"
    return 'stream' + streamnum;
}

// lineNumber is the line number
var lineNumber = 1;

// lineElement returns an element which can be used in the log for the line
function lineElement(line) {
    var tr = meh.elem('tr');

    var numtd = meh.elem('td');
    numtd.appendChild(meh.text(lineNumber.toString()));
    lineNumber++;
    tr.appendChild(numtd);

    var linetd = meh.elem('td');
    linetd.appendChild(meh.text(line.text));
    linetd.classList.add(streamClass(line.stream));
    tr.appendChild(linetd);

    return tr;
}

// start starts the log loader
function start() {
    document.getElementById('log').classList.remove('hidden');
    var tbody = document.getElementById('logbody');
    pbapi.log(hash, (line) => tbody.appendChild(lineElement(line))).then(
        () => displayError('Finished loading log.'),
        (err) => {
            displayError('Failed to load log.');
            var errmsg = meh.elem('h1');
            errmsg.appendChild(meh.text('Failed to load log: '));
            errmsg.appendChild(meh.text(err.toString()));
            document.getElementById('log').replaceWith(errmsg);
        }
    );
}

document.addEventListener("DOMContentLoaded", start);
