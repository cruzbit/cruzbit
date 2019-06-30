var genesisId = '00000000e29a7850088d660489b7b9ae2da763bc3bd83324ecc54eee04840adb';

var tipTime = 0;

function onOpen(ws) {
    ws.send(JSON.stringify({type: 'get_tip_header'}));
}

function onMessage(message, ws) {
    var body = message['body']

    switch (message['type']) {
    case 'inv_block':
	ws.send(JSON.stringify({
	    type: 'get_block_header',
	    body: {
		block_id: body['block_ids'][0],
	    },
	}));
	break;

    case 'tip_header':
	var height = body['header']['height'];
	for (i = height - 9; i <= height; i++) {
	    ws.send(JSON.stringify({
		type: 'get_block_header_by_height',
		body: {
		    height: i,
		},
	    }));
	}
	break;

    case 'block_header':
	tipTime = body['header']['time']

	var blockDiv = document.getElementById(body['block_id']);
	if (blockDiv !== null) {
	    break;
	}
	blockDiv = document.createElement('div')
	blockDiv.setAttribute('id', body['block_id']);
	blockDiv.onclick = function() {
	    ws.send(JSON.stringify({
		type: 'get_block',
		body: {
		    block_id: body['block_id'],
		},
	    }));
	}
	blockDiv.onmouseover = function() {
	    blockDiv.setAttribute('style', 'cursor: pointer;');
	}

	var blockText = document.createTextNode(
	    'block ID: ' +
		body['block_id'] +
		', height: ' +
		body['header']['height']
	);
	blockDiv.appendChild(blockText)

	var blocks = document.getElementById('blocks');
	if (blocks.hasChildNodes()) {
	    var br = document.createElement('br');
	    blocks.insertBefore(br, blocks.firstChild);
	}
	blocks.insertBefore(blockDiv, blocks.firstChild);
	if (blocks.children.length > 19) {
	    blocks.removeChild(blocks.lastChild);
	    blocks.removeChild(blocks.lastChild);
	}
	break;

    case 'block':
	var blockJson = JSON.stringify(body['block'], null, 4);
	var blockText = document.createTextNode(blockJson);
	var blockDiv = document.getElementById(body['block_id']);
	blockDiv.appendChild(document.createElement('br'));
	blockDiv.appendChild(blockText);
	blockDiv.onclick = function() {
	    blockDiv.removeChild(blockDiv.lastChild);
	    blockDiv.removeChild(blockDiv.lastChild);
	    blockDiv.onclick = function() {
		ws.send(JSON.stringify({
		    type: 'get_block',
		    body: {
			block_id: body['block_id'],
		    },
		}));
	    }
	}
	break;
    }
}

function onTick() {
    var now = new Date().getTime() / 1000;
    var since = now - tipTime;

    var hours   = Math.floor(since / 3600);
    var minutes = Math.floor((since - (hours * 3600)) / 60);
    var seconds = Math.floor(since - (hours * 3600) - (minutes * 60));

    if (hours   < 10) {hours   = "0"+hours;}
    if (minutes < 10) {minutes = "0"+minutes;}
    if (seconds < 10) {seconds = "0"+seconds;}

    var clock = document.getElementById('clock');
    clock.innerHTML = 'time since last block: ' + hours + 'h:' + minutes + 'm:' + seconds + 's';
}
