export namespace main {
	
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

