export namespace main {
	
	export class Chat {
	    jid: string;
	    name: string;
	    lastTs: number;
	    tone: string;
	
	    static createFrom(source: any = {}) {
	        return new Chat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jid = source["jid"];
	        this.name = source["name"];
	        this.lastTs = source["lastTs"];
	        this.tone = source["tone"];
	    }
	}
	export class Status {
	    connected: boolean;
	    detail: string;
	    qr: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.detail = source["detail"];
	        this.qr = source["qr"];
	    }
	}

}

