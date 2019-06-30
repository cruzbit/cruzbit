package cruzbit

import (
	"time"

	"github.com/GeertJohan/go.rice/embedded"
)

func init() {

	// define files
	file2 := &embedded.EmbeddedFile{
		Filename:    "index.html",
		FileModTime: time.Unix(1561881241, 0),

		Content: string("<html>\n  <head>\n    <title>cruzbit</title>\n    <style>\n      body { white-space: pre; font-family: monospace; }\n    </style>\n  </head>\n  <body>\n    <h2>cruzbit</h2>\n    <div id=\"blocks\"></div>\n    <div id=\"clock\"></div>\n    <script src=\"status.js\"></script>\n    <script>\n      var ws = new WebSocket(\"wss://\" + location.host + \"/\" + genesisId, [\"cruzbit.1\"]);\n      ws.onopen = function() {\n          onOpen(ws);\n      };\n      ws.onmessage = function(event) {\n          onMessage(JSON.parse(event.data), ws);\n      };\n      setInterval(onTick, 1000);\n    </script>\n  </body>\n</html>\n"),
	}
	file3 := &embedded.EmbeddedFile{
		Filename:    "status.js",
		FileModTime: time.Unix(1561882183, 0),

		Content: string("var genesisId = '00000000e29a7850088d660489b7b9ae2da763bc3bd83324ecc54eee04840adb';\n\nvar tipTime = 0;\n\nfunction onOpen(ws) {\n    ws.send(JSON.stringify({type: 'get_tip_header'}));\n}\n\nfunction onMessage(message, ws) {\n    var body = message['body']\n\n    switch (message['type']) {\n    case 'inv_block':\n\tws.send(JSON.stringify({\n\t    type: 'get_block_header',\n\t    body: {\n\t\tblock_id: body['block_ids'][0], \n\t    },\n\t}));\n\tbreak;\n\n    case 'tip_header':\n\tvar height = body['header']['height'];\n\tfor (i = height - 9; i <= height; i++) {\n\t    ws.send(JSON.stringify({\n\t\ttype: 'get_block_header_by_height',\n\t\tbody: {\n\t\t    height: i,\n\t\t},\n\t    }));\n\t}\n\tbreak;\n\t\n    case 'block_header':\n\ttipTime = body['header']['time']\n\n\tvar blockDiv = document.getElementById(body['block_id']);\n\tif (blockDiv !== null) {\n\t    break;\n\t}\n\tblockDiv = document.createElement('div')\n\tblockDiv.setAttribute('id', body['block_id']);\n\tblockDiv.onclick = function() {\n\t    ws.send(JSON.stringify({\n\t\ttype: 'get_block',\n\t\tbody: {\n\t\t    block_id: body['block_id'],\n\t\t},\n\t    }));\n\t}\n\tblockDiv.onmouseover = function() {\n\t    blockDiv.setAttribute('style', 'cursor: pointer;');\n\t}\n\n\tvar blockText = document.createTextNode(\n\t    'block ID: ' +\n\t\tbody['block_id'] +\n\t\t', height: ' +\n\t\tbody['header']['height']\n\t);\n\tblockDiv.appendChild(blockText)\n\n\tvar blocks = document.getElementById('blocks');\n\tif (blocks.hasChildNodes()) {\n\t    var br = document.createElement('br');\n\t    blocks.insertBefore(br, blocks.firstChild);\n\t}\n\tblocks.insertBefore(blockDiv, blocks.firstChild);\n\tif (blocks.children.length > 19) {\n\t    blocks.removeChild(blocks.lastChild);\n\t    blocks.removeChild(blocks.lastChild);\n\t}\n\tbreak;\n\n    case 'block':\n\tvar blockJson = JSON.stringify(body['block'], null, 4);\n\tvar blockText = document.createTextNode(blockJson);\n\tvar blockDiv = document.getElementById(body['block_id']);\n\tblockDiv.appendChild(document.createElement('br'));\n\tblockDiv.appendChild(blockText);\n\tblockDiv.onclick = function() {\n\t    blockDiv.removeChild(blockDiv.lastChild);\n\t    blockDiv.removeChild(blockDiv.lastChild);\n\t    blockDiv.onclick = function() {\n\t\tws.send(JSON.stringify({\n\t\t    type: 'get_block',\n\t\t    body: {\n\t\t\tblock_id: body['block_id'],\n\t\t    },\n\t\t}));\n\t    }\n\t}\n\tbreak;\n    }\n}\n\nfunction onTick() {\n    var now = new Date().getTime() / 1000;\n    var since = now - tipTime;\n\n    var hours   = Math.floor(since / 3600);\n    var minutes = Math.floor((since - (hours * 3600)) / 60);\n    var seconds = Math.floor(since - (hours * 3600) - (minutes * 60));\n\n    if (hours   < 10) {hours   = \"0\"+hours;}\n    if (minutes < 10) {minutes = \"0\"+minutes;}\n    if (seconds < 10) {seconds = \"0\"+seconds;}\n\n    var clock = document.getElementById('clock');\n    clock.innerHTML = 'time since last block: ' + hours + 'h:' + minutes + 'm:' + seconds + 's';\n}\n"),
	}

	// define dirs
	dir1 := &embedded.EmbeddedDir{
		Filename:   "",
		DirModTime: time.Unix(1561882183, 0),
		ChildFiles: []*embedded.EmbeddedFile{
			file2, // "index.html"
			file3, // "status.js"

		},
	}

	// link ChildDirs
	dir1.ChildDirs = []*embedded.EmbeddedDir{}

	// register embeddedBox
	embedded.RegisterEmbeddedBox(`html`, &embedded.EmbeddedBox{
		Name: `html`,
		Time: time.Unix(1561882183, 0),
		Dirs: map[string]*embedded.EmbeddedDir{
			"": dir1,
		},
		Files: map[string]*embedded.EmbeddedFile{
			"index.html": file2,
			"status.js":  file3,
		},
	})
}
