export namespace main {
	
	export class ConfigResponse {
	    config: Record<string, any>;
	    warnings: string[][];
	
	    static createFrom(source: any = {}) {
	        return new ConfigResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = source["config"];
	        this.warnings = source["warnings"];
	    }
	}
	export class Item {
	    name: string;
	    path: string;
	    source: string;
	    args: string[];
	    history_key?: string;
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.source = source["source"];
	        this.args = source["args"];
	        this.history_key = source["history_key"];
	        this.description = source["description"];
	    }
	}

}

