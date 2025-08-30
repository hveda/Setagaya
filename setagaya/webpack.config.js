const path = require('path');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');

module.exports = (env, argv) => {
    const isProduction = argv.mode === 'production';
    
    return {
        entry: {
            main: './ui/static/js/app.js',
            admin: './ui/static/js/admin.js',
            auth: './ui/static/js/auth.js',
            'file-upload': './ui/static/js/file-upload.js',
            realtime: './ui/static/js/realtime.js'
        },
        
        output: {
            path: path.resolve(__dirname, 'ui/static/dist'),
            filename: isProduction ? '[name].[contenthash].js' : '[name].js',
            clean: true,
        },
        
        module: {
            rules: [
                {
                    test: /\.js$/,
                    exclude: /node_modules/,
                    use: {
                        loader: 'babel-loader',
                        options: {
                            presets: ['@babel/preset-env']
                        }
                    }
                },
                {
                    test: /\.css$/,
                    use: [
                        isProduction ? MiniCssExtractPlugin.loader : 'style-loader',
                        'css-loader'
                    ]
                }
            ]
        },
        
        plugins: [
            ...(isProduction ? [
                new MiniCssExtractPlugin({
                    filename: '[name].[contenthash].css',
                })
            ] : [])
        ],
        
        optimization: {
            splitChunks: {
                chunks: 'all',
                cacheGroups: {
                    vendor: {
                        test: /[\\/]node_modules[\\/]/,
                        name: 'vendors',
                        chunks: 'all',
                    },
                },
            },
        },
        
        devtool: isProduction ? 'source-map' : 'eval-source-map',
        
        resolve: {
            extensions: ['.js', '.json'],
            alias: {
                '@': path.resolve(__dirname, 'ui/static/js'),
            }
        },
        
        externals: {
            'alpinejs': 'Alpine',
            'axios': 'axios'
        }
    };
};