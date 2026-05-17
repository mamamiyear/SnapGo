export namespace domain {
	
	export class S3Config {
	    endpoint: string;
	    region: string;
	    bucket: string;
	    accessKeyId: string;
	    secretAccessKey: string;
	    pathPrefix: string;
	    publicUrlBase: string;
	    usePathStyle: boolean;
	
	    static createFrom(source: any = {}) {
	        return new S3Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpoint = source["endpoint"];
	        this.region = source["region"];
	        this.bucket = source["bucket"];
	        this.accessKeyId = source["accessKeyId"];
	        this.secretAccessKey = source["secretAccessKey"];
	        this.pathPrefix = source["pathPrefix"];
	        this.publicUrlBase = source["publicUrlBase"];
	        this.usePathStyle = source["usePathStyle"];
	    }
	}
	export class AppConfig {
	    hotkey: string;
	    s3: S3Config;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hotkey = source["hotkey"];
	        this.s3 = this.convertValues(source["s3"], S3Config);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class RegionRect {
	    x: number;
	    y: number;
	    w: number;
	    h: number;
	
	    static createFrom(source: any = {}) {
	        return new RegionRect(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.w = source["w"];
	        this.h = source["h"];
	    }
	}

}

