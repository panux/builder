// branch.js is the page management code for /branch.html

// pageURL is the url of the current page
var pageURL = new URL(window.location.href);

// branch is the current branch
var branch = pageURL.searchParams.get('branch') || 'beta';

// changeBranch redirects to a page of the given branch
function changeBranch(branch) {
    // mutate URL
    pageURL.searchParams.set('branch', branch);
    // redirect
    window.location.assign(pageURL.toString());
}

// statusIcons is a map of build states to names of icons to display
var statusIcons = {
    'waiting': 'timer',
    'queued': 'playlist_add',
    'running': 'build',
    'finished': 'done',
    'failed': 'error'
};

// chip generates a Materialize chip containing the given string
function chip(text) {
    var d = meh.div(meh.text(arch));
    d.classList.add('chip');
    return d;
}

// buildElem returns a li element for a build in the list
function buildElem(buildStatus) {
    var li = meh.elem('li');
    li.classList.add('collection-item');

    if(buildinfo.info) {
        li.onclick = () => {
            pageURL.pathname = '/log.html';
            pageURL.searchParams.delete('branch');
            pageURL.searchParams.set('buildHash', buildStatus.info.hash);
            window.location.assign(pageURL.toString());
        };
    }

    var indicator = meh.elem('a');
    indicator.appendChild(meh.icon(statusIcons[buildStatus.state]));
    indicator.classList.add('secondary-content');

    li.appendChild(meh.div(
        meh.text(buildStatus.name),
        chip(buildStatus.arch),
        buildStatus.bootstrap ? chip('bootstrap') : meh.text(''),
        indicator
    ));

    return li;
}

// branchListElem creates the build list
function branchListElem(builds) {
    builds.sort((a, b) => {
        if(a.name != b.name) {
            return a.name > b.name ? 1 : -1;
        }
        if(a.arch != b.arch) {
            return a.arch > b.arch ? 1 : -1;
        }
        return a.bootstrap ? -1 : 1;
    })

    var ul = meh.elem('ul');
    ul.classList.add('collection');

    builds.forEach((bs) => ul.appendChild(buildElem(bs)));

    return ul;
}

// branchListGen returns a Promise to a new build list.
function branchListGen() {
    return new Promise((s, f) => {
        pbapi.branchStatus(branch).then(
            (branchinfo) => {
                var builds = [];
                (new Map(branchinfo.builds)).forEach((v) => builds.push(v));

                s(buildListElem(builds));
            },
            f,
        );
    })
}

// start starts the build list updater.
function start() {
    var elem = document.getElementById('list');
    var prevFinished = true;
    setInterval(() => {
        document.getElementById('list').appendChild(meh.loadingWheel);

        if(!prevFinished) {
            meh.toast(meh.text('Slow connection. Updates may be delayed.'));
            return;
        }
        branchListGen().then(
            (l) => {
                elem.replaceWith(l);
                elem = l;
            },
            () => meh.toast(meh.text('Failed to update. Retrying soon.'))
        );
    }, 10000);
}

//run start on content loaded
document.addEventListener("DOMContentLoaded", start);
