// pbapi.js contains an API for the pbuild server
var pbapi = {};

// pbapi.branchStatus gets the status of a branch (see BranchStatus in pbuild).
pbapi.branchStatus = meh.getJSONRequest('/api/branch', 'branch');

// internal use only
pbapi.logurlqg = meh.urlquery('/api/log', 'buildhash');

// pbapi.log reads a log stream.
pbapi.log = (buildinfo, linecallback) => {
    return new Promise((s, f) => {
        try {
            var logev = new EventSource(pbapi.logurlqg(buildinfo.hash));
            logev.addEventListener('error', () => {
                logev.close();
                f('EventSource failed.');
            });
            logev.addEventListener('terminate', (msg) => {
                logev.close();
                if(msg.data === 'EOF') {
                    s();
                } else {
                    f(msg.data);
                }
            });
            logev.addEventListener('log', (msg) => {
                try {
                    linecallback(JSON.parse(msg.data));
                } catch(error) {
                    logev.close();
                    f(error);
                }
            });
        } catch(error) {
            f(error);
        }
    });
};
